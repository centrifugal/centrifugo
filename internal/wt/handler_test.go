package wt_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/middleware"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/require"
)

func TestWebTransportWithLogMiddleware(t *testing.T) {
	cert, err := tls.LoadX509KeyPair("../../tmp/localhost+2.pem", "../../tmp/localhost+2-key.pem")
	require.NoError(t, err)

	wtServer := &webtransport.Server{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/wt", func(w http.ResponseWriter, r *http.Request) {
		session, err := wtServer.Upgrade(w, r)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		stream, err := session.AcceptStream(r.Context())
		if err != nil {
			t.Logf("accept stream error: %v", err)
			return
		}
		data, err := io.ReadAll(stream)
		if err != nil {
			t.Logf("read error: %v", err)
			return
		}
		_, _ = stream.Write(data)
		_ = stream.Close()
	})

	// Wrap with LogRequest middleware — this is the key part under test.
	wtServer.H3 = http3.Server{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{http3.NextProtoH3},
		},
		Handler: middleware.LogRequest(mux),
	}

	udpAddr, err := net.ResolveUDPAddr("udp", "localhost:0")
	require.NoError(t, err)
	udpConn, err := net.ListenUDP("udp", udpAddr)
	require.NoError(t, err)
	port := udpConn.LocalAddr().(*net.UDPAddr).Port

	servErr := make(chan error, 1)
	go func() { servErr <- wtServer.Serve(udpConn) }()
	defer func() {
		require.NoError(t, wtServer.Close())
		<-servErr
	}()

	d := webtransport.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		QUICConfig:      &quic.Config{EnableDatagrams: true},
	}
	defer func() {
		_ = d.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rsp, session, err := d.Dial(ctx, fmt.Sprintf("https://localhost:%d/wt", port), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)
	defer func() {
		_ = session.CloseWithError(0, "")
	}()

	stream, err := session.OpenStream()
	require.NoError(t, err)
	_ = stream.SetDeadline(time.Now().Add(5 * time.Second))

	msg := []byte("hello webtransport")
	_, err = stream.Write(msg)
	require.NoError(t, err)
	require.NoError(t, stream.Close())

	reply, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.Equal(t, msg, reply)
}
