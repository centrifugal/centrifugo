package app

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cristalhq/jwt/v5"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/require"
)

// writeTestCert writes a self-signed certificate for localhost and its key
// as PEM files, returning their paths.
func writeTestCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0600))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600))
	return certPath, keyPath
}

// startWebTransportApp starts an App serving WebTransport over HTTP/3 with TLS
// and returns its UDP address.
func startWebTransportApp(t *testing.T) string {
	t.Helper()
	certPath, keyPath := writeTestCert(t)
	cfg := fmt.Sprintf(`{
	"client": {"token": {"hmac_secret_key": "secret"}},
	"http_server": {
		"tls": {"enabled": true, "cert_pem": %q, "key_pem": %q},
		"http3": {"enabled": true}
	},
	"webtransport": {"enabled": true},
	"usage_stats": {"disabled": true}
}`, certPath, keyPath)
	a, err := newTestApp(t, cfg)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	t.Cleanup(func() { _, _ = a.Shutdown(ExitStop, 0) })
	for _, addr := range a.Addrs() {
		if udp, ok := addr.(*net.UDPAddr); ok {
			return fmt.Sprintf("127.0.0.1:%d", udp.Port)
		}
	}
	t.Fatal("no UDP address")
	return ""
}

// dialWebTransport opens a WebTransport session to addr as a browser would:
// offering the h3 ALPN.
func dialWebTransport(t *testing.T, addr string) *webtransport.Session {
	t.Helper()
	d := &webtransport.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}},
		QUICConfig:      &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: true},
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rsp, session, err := d.Dial(ctx, "https://"+addr+"/connection/webtransport", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)
	require.Equal(t, http3.NextProtoH3, session.SessionState().ConnectionState.TLS.NegotiatedProtocol)
	t.Cleanup(func() { _ = session.CloseWithError(0, "") })
	return session
}

// wtReply is a reply or push read from a JSON WebTransport stream.
type wtReply struct {
	ID    uint32 `json:"id"`
	Error *struct {
		Code uint32 `json:"code"`
	} `json:"error"`
	Push *struct {
		Disconnect *struct {
			Code uint32 `json:"code"`
		} `json:"disconnect"`
	} `json:"push"`
}

func (r wtReply) disconnectCode() (uint32, bool) {
	if r.Push == nil || r.Push.Disconnect == nil {
		return 0, false
	}
	return r.Push.Disconnect.Code, true
}

// connectWebTransport sends a connect with token over a new stream of session
// and returns what the server sent until done returns true for a reply or the
// stream ended.
func connectWebTransport(t *testing.T, session *webtransport.Session, token string, done func(wtReply) bool) []wtReply {
	t.Helper()
	stream, err := session.OpenStream()
	require.NoError(t, err)
	require.NoError(t, stream.SetDeadline(time.Now().Add(10*time.Second)))
	cmd, err := json.Marshal(map[string]any{"id": 1, "connect": map[string]string{"token": token}})
	require.NoError(t, err)
	_, err = stream.Write(append(cmd, '\n'))
	require.NoError(t, err)

	var replies []wtReply
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var r wtReply
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r), string(scanner.Bytes()))
		replies = append(replies, r)
		if done(r) {
			break
		}
	}
	return replies
}

func connectionToken(t *testing.T, exp time.Time) string {
	t.Helper()
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte("secret"))
	require.NoError(t, err)
	token, err := jwt.NewBuilder(signer).Build(map[string]any{"sub": "user", "exp": exp.Unix()})
	require.NoError(t, err)
	return token.String()
}

func TestWebTransportConnectRefused(t *testing.T) {
	addr := startWebTransportApp(t)

	t.Run("expired token", func(t *testing.T) {
		session := dialWebTransport(t, addr)
		replies := connectWebTransport(t, session, connectionToken(t, time.Now().Add(-time.Minute)), func(r wtReply) bool {
			return r.ID == 1
		})
		require.Len(t, replies, 1)
		require.NotNil(t, replies[0].Error)
		require.Equal(t, uint32(109), replies[0].Error.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		session := dialWebTransport(t, addr)
		replies := connectWebTransport(t, session, "invalid", func(r wtReply) bool {
			_, ok := r.disconnectCode()
			return ok
		})
		require.NotEmpty(t, replies)
		code, ok := replies[len(replies)-1].disconnectCode()
		require.True(t, ok, "stream ended without a disconnect push")
		require.Equal(t, uint32(3500), code)
	})
}
