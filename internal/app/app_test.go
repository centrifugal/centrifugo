package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// testConfig with CENTRIFUGO_HTTP_SERVER_PORT=0 (set by newTestApp: a zero
// port in the file is replaced by the default one) binds a free port.
const testConfig = `{
	"client": {"token": {"hmac_secret_key": "secret"}},
	"http_api": {"key": "api-key"},
	"uni_http_stream": {"enabled": true},
	"usage_stats": {"disabled": true}
}`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

// newTestApp builds an App from configJSON.
func newTestApp(t *testing.T, configJSON string, extensions ...Extension) (*App, error) {
	t.Helper()
	t.Setenv("CENTRIFUGO_HTTP_SERVER_PORT", "0")
	opts := Options{Extensions: extensions}
	a, err := New(Centrifugo(opts), writeConfig(t, configJSON), opts)
	if err != nil {
		return nil, err
	}
	t.Cleanup(a.Close)
	return a, nil
}

// recorder is an extension implementing every optional interface and
// recording the calls.
type recorder struct {
	mu    sync.Mutex
	calls []string

	setup func(c *Components) error
	mux   func(m *MuxContext) error
}

func (r *recorder) record(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *recorder) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recorder) Name() string { return "recorder" }

func (r *recorder) LogWriters(config.Config) ([]io.Writer, func(), error) {
	r.record("LogWriters")
	return nil, func() { r.record("closeLogWriters") }, nil
}

func (r *recorder) Setup(c *Components) error {
	r.record("Setup")
	if r.setup != nil {
		return r.setup(c)
	}
	return nil
}

func (r *recorder) ExtendMux(m *MuxContext) error {
	r.record("ExtendMux")
	if r.mux != nil {
		return r.mux(m)
	}
	return nil
}

func (r *recorder) ConfigChangeRejected(err error) {
	r.record("ConfigChangeRejected:" + err.Error())
}

func (r *recorder) ConfigReloaded(err error) {
	if err != nil {
		r.record("ConfigReloaded:" + err.Error())
		return
	}
	r.record("ConfigReloaded")
}

var (
	_ LogWriterProvider = (*recorder)(nil)
	_ Setupper          = (*recorder)(nil)
	_ MuxExtender       = (*recorder)(nil)
	_ ConfigObserver    = (*recorder)(nil)
)

func TestExtensionPhaseOrder(t *testing.T) {
	r := &recorder{}
	r.setup = func(c *Components) error {
		require.NotNil(t, c.Node)
		require.NotNil(t, c.ClientHandler)
		require.NotNil(t, c.Lifecycle())
		require.NotNil(t, c.HTTPExecutor)
		require.NotNil(t, c.GRPCExecutor)
		require.NotNil(t, c.ConsumingExecutor)
		// The client handler registers its node callbacks after Setup: this
		// handler is replaced, so connections below go through the client handler.
		c.Node.OnConnecting(func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
			return centrifuge.ConnectReply{}, errors.New("registered before the client handler")
		})
		return nil
	}
	a, err := newTestApp(t, testConfig, r)
	require.NoError(t, err)
	want := []string{"LogWriters", "Setup"}
	require.Equal(t, want, r.Calls())
	n := len(want)

	require.NoError(t, a.Start())
	require.Equal(t, []string{"ExtendMux"}, r.Calls()[n:])

	addr := httpAddr(t, a)
	stream := connectUniHTTPStream(t, addr, "user", "test")
	defer func() { _ = stream.Close() }()

	a.reload()
	require.Equal(t, []string{"ConfigReloaded"}, r.Calls()[n+1:])

	a.ConfigChangeRejected(errors.New("bad change"))
	require.Equal(t, "ConfigChangeRejected:bad change", r.Calls()[n+2])

	code, err := a.Shutdown(ExitStop, 0)
	require.NoError(t, err)
	require.Equal(t, 0, code)
	a.Close()
	require.Equal(t, "closeLogWriters", r.Calls()[len(r.Calls())-1])
}

func TestRouteConflict(t *testing.T) {
	r := &recorder{mux: func(m *MuxContext) error {
		m.AddRoute(Route{Name: "custom", Pattern: "/connection/uni_http_stream", Handler: http.NotFoundHandler()})
		return nil
	}}
	a, err := newTestApp(t, testConfig, r)
	require.NoError(t, err)
	err = a.Start()
	require.ErrorContains(t, err, `pattern "/connection/uni_http_stream" is already registered`)

	// Two extension routes with one pattern conflict too.
	r = &recorder{mux: func(m *MuxContext) error {
		m.AddRoute(Route{Name: "a", Pattern: "/custom", Handler: http.NotFoundHandler()})
		m.AddRoute(Route{Name: "b", Pattern: "/custom", Handler: http.NotFoundHandler()})
		return nil
	}}
	a, err = newTestApp(t, testConfig, r)
	require.NoError(t, err)
	require.ErrorContains(t, a.Start(), `route b: pattern "/custom" is already registered`)

	// A pattern http.ServeMux rejects is an error, not a panic.
	r = &recorder{mux: func(m *MuxContext) error {
		m.AddRoute(Route{Name: "bad", Pattern: "/{x", Handler: http.NotFoundHandler()})
		return nil
	}}
	a, err = newTestApp(t, testConfig, r)
	require.NoError(t, err)
	require.ErrorContains(t, a.Start(), "route bad")
}

func TestRouteIfAbsent(t *testing.T) {
	handler := func(body string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		})
	}
	r := &recorder{mux: func(m *MuxContext) error {
		// Taken by a built-in route: skipped.
		m.AddRoute(Route{Name: "fallback-api", Pattern: "/api", Handler: handler("fallback"), IfAbsent: true})
		// Free: registered.
		m.AddRoute(Route{Name: "fallback-free", Pattern: "/free", Handler: handler("free"), IfAbsent: true})
		// Taken by the regular route below, which is registered first.
		m.AddRoute(Route{Name: "fallback-custom", Pattern: "/custom", Handler: handler("fallback"), IfAbsent: true})
		m.AddRoute(Route{Name: "custom", Pattern: "/custom", Handler: handler("custom"), Chain: ChainNone})
		return nil
	}}
	a, err := newTestApp(t, testConfig, r)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	addr := httpAddr(t, a)

	get := func(path string) (int, string) {
		resp, err := http.Get("http://" + addr + path)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(body)
	}
	code, body := get("/free")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "free", body)
	_, body = get("/custom")
	require.Equal(t, "custom", body)
	code, _ = get("/api")
	require.Equal(t, http.StatusMethodNotAllowed, code) // Built-in API handler accepts POST only.
}

// Extension routes make a server start on an address with no built-in
// handlers. IfAbsent routes don't.
func TestRouteStartsServer(t *testing.T) {
	// The internal server on its own free port.
	cfg := `{
		"http_server": {"internal_address": "127.0.0.1", "internal_port": "0"},
		"http_api": {"disabled": true},
		"usage_stats": {"disabled": true}
	}`
	r := &recorder{mux: func(m *MuxContext) error {
		m.AddRoute(Route{Name: "internal-fallback", Port: PortInternal, Pattern: "/fallback", Handler: http.NotFoundHandler(), IfAbsent: true})
		return nil
	}}
	a, err := newTestApp(t, cfg, r)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	require.Len(t, a.Addrs(), 1) // External only.

	r = &recorder{mux: func(m *MuxContext) error {
		m.AddRoute(Route{Name: "internal", Port: PortInternal, Pattern: "/internal", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})})
		return nil
	}}
	a, err = newTestApp(t, cfg, r)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	require.Len(t, a.Addrs(), 2)
	var internalAddr string
	for _, addr := range a.Addrs() {
		if host, _, _ := net.SplitHostPort(addr.String()); host == "127.0.0.1" {
			internalAddr = addr.String()
		}
	}
	resp, err := http.Get("http://" + internalAddr + "/internal")
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusTeapot, resp.StatusCode)
}

func TestLifecycleExitBeforeWait(t *testing.T) {
	a, err := newTestApp(t, testConfig)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	a.Lifecycle().Exit(3, errors.New("stop"))
	reason, code := a.Wait(nil)
	require.Equal(t, ExitRequested, reason)
	require.Equal(t, 3, code)
	code, err = a.Shutdown(reason, code)
	require.NoError(t, err)
	require.Equal(t, 3, code)
}

func TestLifecycleExitDuringNew(t *testing.T) {
	r := &recorder{setup: func(c *Components) error {
		c.Lifecycle().Exit(5, nil)
		return nil
	}}
	_, err := newTestApp(t, testConfig, r)
	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 5, exitCode(err))
}

func TestWaitRestart(t *testing.T) {
	a, err := newTestApp(t, testConfig)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	restart := make(chan struct{})
	close(restart)
	reason, code := a.Wait(restart)
	require.Equal(t, ExitRestart, reason)
	require.Equal(t, 0, code)
}

func TestLifecycleStopStepsAndProducers(t *testing.T) {
	var order []string
	var mu sync.Mutex
	add := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}
	r := &recorder{setup: func(c *Components) error {
		lc := c.Lifecycle()
		lc.OnStop("first", func(context.Context) error { add("stop first"); return nil })
		lc.OnStop("second", func(context.Context) error { add("stop second"); return errors.New("failed") })
		require.True(t, lc.Go("producer", func(ctx context.Context) {
			<-ctx.Done()
			add("producer done")
		}))
		return nil
	}}
	a, err := newTestApp(t, testConfig, r)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	_, err = a.Shutdown(ExitStop, 0)
	require.ErrorContains(t, err, "second: failed")
	require.Equal(t, []string{"producer done", "stop second", "stop first"}, order)
	require.False(t, a.Lifecycle().Go("late", func(context.Context) {}))
}

// An App serves a client connection and a server API publish, then shuts down.
func TestAppServe(t *testing.T) {
	a, err := newTestApp(t, testConfig)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	addr := httpAddr(t, a)

	stream := connectUniHTTPStream(t, addr, "user", "test")
	defer func() { _ = stream.Close() }()

	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/api/publish", strings.NewReader(`{"channel":"test","data":{"text":"hello"}}`))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "api-key")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotContains(t, string(body), "error")

	push := stream.next(t)
	require.Equal(t, "test", push.Channel)
	require.JSONEq(t, `{"text":"hello"}`, string(push.Pub.Data))

	code, err := a.Shutdown(ExitStop, 0)
	require.NoError(t, err)
	require.Equal(t, 0, code)
	a.Close()
}

func httpAddr(t *testing.T, a *App) string {
	t.Helper()
	addrs := a.Addrs()
	require.NotEmpty(t, addrs)
	_, port, err := net.SplitHostPort(addrs[0].String())
	require.NoError(t, err)
	return "127.0.0.1:" + port
}

type uniStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
}

type uniPush struct {
	Channel string `json:"channel"`
	Pub     struct {
		Data json.RawMessage `json:"data"`
	} `json:"pub"`
	Connect json.RawMessage `json:"connect"`
}

func (s *uniStream) next(t *testing.T) uniPush {
	t.Helper()
	require.True(t, s.scanner.Scan(), "stream ended: %v", s.scanner.Err())
	var p uniPush
	require.NoError(t, json.Unmarshal(s.scanner.Bytes(), &p))
	return p
}

func (s *uniStream) Close() error {
	return s.body.Close()
}

// connectUniHTTPStream connects a unidirectional HTTP stream client with a
// token subscribing it to channel, and reads the connect push.
func connectUniHTTPStream(t *testing.T, addr string, user string, channel string) *uniStream {
	t.Helper()
	signer, err := jwt.NewSignerHS(jwt.HS256, []byte("secret"))
	require.NoError(t, err)
	token, err := jwt.NewBuilder(signer).Build(map[string]any{
		"sub":      user,
		"exp":      time.Now().Add(time.Hour).Unix(),
		"channels": []string{channel},
	})
	require.NoError(t, err)
	body, err := json.Marshal(map[string]string{"token": token.String()})
	require.NoError(t, err)
	resp, err := http.Post("http://"+addr+"/connection/uni_http_stream", "application/json", strings.NewReader(string(body)))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	s := &uniStream{body: resp.Body, scanner: bufio.NewScanner(resp.Body)}
	push := s.next(t)
	require.NotEmpty(t, push.Connect)
	return s
}

func TestRegistrationAfterReadFails(t *testing.T) {
	var muxCtx *MuxContext
	r := &recorder{
		mux: func(m *MuxContext) error {
			muxCtx = m
			return nil
		},
	}
	a, err := newTestApp(t, testConfig, r)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	// Start read the routes MuxContext registered: a later AddRoute panics.
	require.PanicsWithValue(t, "MuxContext.AddRoute called after ExtendMux: it must be called before", func() {
		muxCtx.AddRoute(Route{Pattern: "/late", Handler: http.NotFoundHandler()})
	})
}
