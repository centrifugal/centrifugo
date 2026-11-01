package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// leakOptions are the goroutines the leak tests ignore, besides those
// running when a test starts.
func leakOptions() []goleak.Option {
	return []goleak.Option{
		goleak.IgnoreCurrent(),
		// autocert.Manager renewal timers have no stop API.
		goleak.IgnoreTopFunction("golang.org/x/crypto/acme/autocert.(*domainRenewal).renew"),
		// A centrifuge node job unsubscribing an emptied channel from the
		// broker after a delay: node.Shutdown doesn't wait for it, it returns
		// after the delay.
		goleak.IgnoreAnyFunction("github.com/centrifugal/centrifuge.(*Node).unsubscribeFromBrokerWhenEmpty.func1"),
	}
}

// leakTestConfig serves connections, the server API (HTTP and gRPC),
// unidirectional gRPC and metrics, and exports metrics to Graphite. The gRPC
// ports are set by leakConfig.
const leakTestConfig = `{
	"client": {"token": {"hmac_secret_key": "secret"}},
	"http_api": {"key": "api-key"},
	"grpc_api": {"enabled": true},
	"uni_grpc": {"enabled": true},
	"uni_http_stream": {"enabled": true},
	"prometheus": {"enabled": true},
	"graphite": {"enabled": true, "host": "127.0.0.1", "port": 1, "interval": "10ms"},
	"usage_stats": {"disabled": true}
}`

// leakConfig is leakTestConfig, with the gRPC servers binding free ports.
func leakConfig(t *testing.T) string {
	t.Helper()
	setGRPCPortsFree(t)
	return leakTestConfig
}

// setGRPCPortsFree makes the gRPC servers bind free ports (a zero port in the
// config file is replaced by the default one).
func setGRPCPortsFree(t *testing.T) {
	t.Helper()
	t.Setenv("CENTRIFUGO_GRPC_API_PORT", "0")
	t.Setenv("CENTRIFUGO_UNI_GRPC_PORT", "0")
}

// testHTTPClient is a client whose idle connections the tests close.
func testHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	tr := &http.Transport{}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr}
}

// apiPublish publishes data to channel through the HTTP server API.
func apiPublish(t *testing.T, c *http.Client, addr string, channel string, data string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/api/publish",
		strings.NewReader(`{"channel":"`+channel+`","data":`+data+`}`))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "api-key")
	resp, err := c.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotContains(t, string(body), "error")
}

// publishCount returns the number of HTTP API publish commands in /metrics.
func publishCount(t *testing.T, c *http.Client, addr string) float64 {
	t.Helper()
	resp, err := c.Get("http://" + addr + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "centrifugo_api_command_duration_seconds_histogram_count{") &&
			strings.Contains(line, `method="publish"`) && strings.Contains(line, `protocol="http"`) {
			fields := strings.Fields(line)
			v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
			require.NoError(t, err)
			return v
		}
	}
	return 0
}

// serveTraffic connects a client, publishes to it through the server API,
// reads the publication and waits until the API publish is in the metrics of
// the App, which start empty for each App.
func serveTraffic(t *testing.T, a *App) {
	t.Helper()
	serveTrafficAt(t, httpAddr(t, a))
}

// serveTrafficAt is serveTraffic for the App serving on addr.
func serveTrafficAt(t *testing.T, addr string) {
	t.Helper()
	c := testHTTPClient(t)
	stream := connectUniHTTPStream(t, addr, "user", "test")
	defer func() { _ = stream.Close() }()
	apiPublish(t, c, addr, "test", `{"text":"hello"}`)
	push := stream.next(t)
	require.Equal(t, "test", push.Channel)
	var count float64
	require.Eventually(t, func() bool {
		count = publishCount(t, c, addr)
		return count > 0
	}, 5*time.Second, 10*time.Millisecond, "metrics updated")
	require.Equal(t, float64(1), count)
}

// Two Apps run one after the other in one process with real traffic and
// leave nothing running; a third binds the port of the first.
func TestLifecycleTwoRunsNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	var port string
	for i := 0; i < 2; i++ {
		a, err := newTestApp(t, leakConfig(t))
		require.NoError(t, err)
		require.NoError(t, a.Start())
		if port == "" {
			_, port, err = net.SplitHostPort(a.Addrs()[0].String())
			require.NoError(t, err)
		}
		// Metrics keep updating in the second run, in its own registry.
		serveTraffic(t, a)
		code, err := a.Shutdown(ExitStop, 0)
		require.NoError(t, err)
		require.Equal(t, 0, code)
		a.Close()
	}

	t.Setenv("CENTRIFUGO_HTTP_SERVER_PORT", port)
	opts := Options{}
	a, err := New(Centrifugo(opts), writeConfig(t, leakConfig(t)), opts)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	serveTraffic(t, a)
	a.Close()
}

// failingExtension fails the phase named by failAt, after registering a stop
// step and starting a producer which both must be stopped.
type failingExtension struct {
	failAt  string
	stopped bool
}

func (e *failingExtension) Name() string { return "failing" }

func (e *failingExtension) fail(phase string, lc *Lifecycle) error {
	if lc != nil && !e.stopped {
		e.stopped = true
		lc.OnStop("failing extension", func(context.Context) error { return nil })
		lc.Go("failing extension producer", func(ctx context.Context) { <-ctx.Done() })
	}
	if phase == e.failAt {
		return errors.New("failed at " + phase)
	}
	return nil
}

func (e *failingExtension) LogWriters(config.Config) ([]io.Writer, func(), error) {
	return nil, nil, e.fail("LogWriters", nil)
}

func (e *failingExtension) Setup(c *Components) error { return e.fail("Setup", c.Lifecycle()) }

func (e *failingExtension) ExtendMux(m *MuxContext) error { return e.fail("ExtendMux", m.Lifecycle) }

// failingPhase makes New fail in a phase by config, after the earlier phases
// created what must be released: client is added to the client section,
// extra to the top level; err is in the error of New.
type failingPhase struct {
	client string
	extra  string
	err    string
}

var failingPhaseConfigs = map[string]failingPhase{
	// Phase 3: the config is invalid.
	"config": {
		extra: `, "channel": {"namespaces": [{"name": "bad name"}]}`,
		err:   "error validating config",
	},
	// Phase 4: the connect proxy is built, then the HTTP subscribe stream
	// proxy fails the proxy map.
	"proxies": {
		client: `, "proxy": {"connect": {"enabled": true, "endpoint": "grpc://127.0.0.1:1"}}`,
		extra: `, "channel": {"without_namespace": {"subscribe_stream_proxy_enabled": true},
			"proxy": {"subscribe_stream": {"endpoint": "http://127.0.0.1:1"}}}`,
		err: "only GRPC endpoints supported",
	},
	// Phase 6: the Postgres broker can't initialize its schema.
	"engines": {
		extra: `, "broker": {"enabled": true, "type": "postgres",
			"postgres": {"dsn": "postgres://test:test@127.0.0.1:1/test"}}`,
		err: "Postgres stream broker schema",
	},
	// Phase 8: the consumer can't connect.
	"consumers": {
		extra: `, "consumers": [{"name": "pg", "type": "postgresql", "enabled": true,
			"postgresql": {"dsn": "postgres://test:test@127.0.0.1:1/test", "outbox_table_name": "outbox"}}]`,
		err: "error initializing consumers",
	},
}

// A New failing in any phase, and a Start failing before it binds, leave
// nothing running and remove the pid file.
func TestLifecycleFailingPhasesNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	for _, phase := range []string{
		"LogWriters", "config", "proxies", "engines", "Setup", "consumers", "ExtendMux",
	} {
		t.Run(phase, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "centrifugo.pid")
			setGRPCPortsFree(t)
			fp, byConfig := failingPhaseConfigs[phase]
			cfg := `{
				"client": {"token": {"hmac_secret_key": "secret"}` + fp.client + `},
				"http_api": {"key": "api-key"},
				"grpc_api": {"enabled": true},
				"usage_stats": {"disabled": true},
				"pid_file": "` + pidFile + `"` + fp.extra + `}`
			ext := &failingExtension{failAt: phase}
			a, err := newTestApp(t, cfg, ext)
			if phase == "ExtendMux" {
				require.NoError(t, err)
				err = a.Start()
				require.ErrorContains(t, err, "failed at "+phase)
				_, err = a.Shutdown(ExitRequested, exitCode(err))
				require.NoError(t, err)
				a.Close()
			} else if byConfig {
				require.ErrorContains(t, err, fp.err)
			} else {
				require.ErrorContains(t, err, "failed at "+phase)
			}
			_, statErr := os.Stat(pidFile)
			require.True(t, os.IsNotExist(statErr), "pid file removed")
		})
	}
}

// A Start failing after it bound listeners closes them.
func TestLifecycleStartFailsAfterBinding(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = busy.Close() }()
	_, busyPort, err := net.SplitHostPort(busy.Addr().String())
	require.NoError(t, err)

	// HTTP is bound first, then the gRPC API fails to bind.
	cfg := `{
		"client": {"token": {"hmac_secret_key": "secret"}},
		"http_api": {"key": "api-key"},
		"grpc_api": {"enabled": true, "address": "127.0.0.1", "port": ` + busyPort + `},
		"usage_stats": {"disabled": true}
	}`
	t.Setenv("CENTRIFUGO_HTTP_SERVER_PORT", "0")
	opts := Options{}
	a, err := New(Centrifugo(opts), writeConfig(t, cfg), opts)
	require.NoError(t, err)
	require.ErrorContains(t, a.Start(), "cannot listen")
	require.Len(t, a.Addrs(), 1) // HTTP was bound before the failure.
	_, httpPort, err := net.SplitHostPort(a.Addrs()[0].String())
	require.NoError(t, err)
	_, err = a.Shutdown(ExitRequested, 1)
	require.NoError(t, err)
	a.Close()

	// The HTTP port is free again.
	ln, err := net.Listen("tcp", ":"+httpPort)
	require.NoError(t, err)
	require.NoError(t, ln.Close())
}

// waitingExtension waits in ExtendMux for shutdown to begin.
type waitingExtension struct {
	waiting chan struct{}
}

func (e *waitingExtension) Name() string { return "waiting" }

func (e *waitingExtension) ExtendMux(m *MuxContext) error {
	close(e.waiting)
	<-m.Lifecycle.Context().Done()
	return nil
}

// Shutdown during Start releases what waits in Start, and Start returns.
func TestLifecycleShutdownDuringStart(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	ext := &waitingExtension{waiting: make(chan struct{})}
	a, err := newTestApp(t, leakConfig(t), ext)
	require.NoError(t, err)
	startErr := make(chan error, 1)
	go func() { startErr <- a.Start() }()
	<-ext.waiting
	code, err := a.Shutdown(ExitStop, 0)
	require.NoError(t, err)
	require.Equal(t, 0, code)
	require.ErrorIs(t, <-startErr, errShutdownDuringStart)
	a.Close()
}

// logRecorder is an extension receiving every log line.
type logRecorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *logRecorder) Name() string { return "logs" }

func (r *logRecorder) LogWriters(config.Config) ([]io.Writer, func(), error) {
	return []io.Writer{r}, nil, nil
}

func (r *logRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *logRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

// hangingExtension registers a stop step which hangs until released, between
// two which return.
type hangingExtension struct {
	logRecorder
	release chan struct{}
	ran     sync.Map
}

func (e *hangingExtension) Setup(c *Components) error {
	lc := c.Lifecycle()
	lc.OnStop("before hang", func(context.Context) error { e.ran.Store("before hang", true); return nil })
	lc.OnStop("hang", func(context.Context) error { <-e.release; return nil })
	lc.OnStop("after hang", func(context.Context) error { e.ran.Store("after hang", true); return nil })
	return nil
}

// A stop step which hangs is logged by name and left running, the later
// steps still run, and the process exits within shutdown.timeout with code 1,
// without the backstop.
func TestLifecycleHangingStopStep(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	ext := &hangingExtension{release: make(chan struct{})}
	closed := make(chan struct{})
	defer func() {
		close(ext.release)
		// Close may be left running when it didn't return in time; it
		// restores the global logger, which the next test sets up.
		<-closed
	}()
	cfg := `{
		"client": {"token": {"hmac_secret_key": "secret"}},
		"shutdown": {"timeout": "500ms"},
		"usage_stats": {"disabled": true}
	}`
	t.Setenv("CENTRIFUGO_HTTP_SERVER_PORT", "0")
	opts := Options{Extensions: []Extension{ext, exitExtension{at: "route", code: 0}}}
	done := make(chan []int, 1)
	built := make(chan *App, 1)
	go func() {
		var codes []int
		run(Centrifugo(opts), writeConfig(t, cfg), opts, process{
			exit:   func(code int) { codes = append(codes, code) },
			closed: func() { close(closed) },
			built:  func(a *App) { built <- a },
		})
		done <- codes
	}()
	requestExit(t, servingAddr(t, built))
	started := time.Now()
	codes := <-done
	elapsed := time.Since(started)

	require.Equal(t, []int{1}, codes)
	require.Less(t, elapsed, 500*time.Millisecond+stopTolerance)
	_, ok := ext.ran.Load("before hang")
	require.True(t, ok)
	_, ok = ext.ran.Load("after hang")
	require.True(t, ok)
	logs := ext.String()
	require.Contains(t, logs, `"step":"hang"`)
	require.Contains(t, logs, "shutdown step did not finish in time")
	require.Contains(t, logs, "shutdown timeout reached")
}

// stopTolerance is how much later than shutdown.timeout the tests accept a
// stop to end.
const stopTolerance = 200 * time.Millisecond

// hangingEverywhere has two stop steps and a log writer closer which hang
// until released.
type hangingEverywhere struct {
	logRecorder
	release chan struct{}
}

func (e *hangingEverywhere) LogWriters(config.Config) ([]io.Writer, func(), error) {
	return []io.Writer{&e.logRecorder}, func() { <-e.release }, nil
}

func (e *hangingEverywhere) Setup(c *Components) error {
	lc := c.Lifecycle()
	lc.OnStop("hang 1", func(context.Context) error { <-e.release; return nil })
	lc.OnStop("hang 2", func(context.Context) error { <-e.release; return nil })
	return nil
}

// The whole stop, the stop steps remaining after the deadline and Close
// included, ends within shutdown.timeout, even when they all hang.
func TestRunStopWithinShutdownTimeout(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	ext := &hangingEverywhere{release: make(chan struct{})}
	closed := make(chan struct{})
	defer func() {
		// Close, left running, restores the global logger once released.
		close(ext.release)
		<-closed
	}()
	cfg := `{
		"client": {"token": {"hmac_secret_key": "secret"}},
		"shutdown": {"timeout": "1s"},
		"usage_stats": {"disabled": true}
	}`
	t.Setenv("CENTRIFUGO_HTTP_SERVER_PORT", "0")
	opts := Options{Extensions: []Extension{ext, exitExtension{at: "route", code: 0}}}
	done := make(chan []int, 1)
	built := make(chan *App, 1)
	go func() {
		var codes []int
		run(Centrifugo(opts), writeConfig(t, cfg), opts, process{
			exit:   func(code int) { codes = append(codes, code) },
			closed: func() { close(closed) },
			built:  func(a *App) { built <- a },
		})
		done <- codes
	}()
	requestExit(t, servingAddr(t, built))
	started := time.Now()
	codes := <-done
	elapsed := time.Since(started)

	require.Equal(t, []int{1}, codes)
	require.Less(t, elapsed, time.Second+stopTolerance)
	logs := ext.String()
	require.Contains(t, logs, `"step":"hang 2"`)
	require.Contains(t, logs, `"step":"hang 1"`)
	require.Contains(t, logs, "shutdown timeout reached")
}

func TestStopGrace(t *testing.T) {
	for _, tc := range []struct {
		timeout, grace, closeBudget time.Duration
	}{
		{30 * time.Second, 5 * time.Second, time.Second},
		{10 * time.Second, 2 * time.Second, 400 * time.Millisecond},
		{500 * time.Millisecond, 100 * time.Millisecond, 20 * time.Millisecond},
	} {
		grace, closeBudget := stopGrace(tc.timeout)
		require.Equal(t, tc.grace, grace, tc.timeout)
		require.Equal(t, tc.closeBudget, closeBudget, tc.timeout)
	}
}

// exitExtension calls Lifecycle.Exit in the phase named by at; "route" calls
// it from a request to /exit once the App serves.
type exitExtension struct {
	at   string
	code int
}

func (e exitExtension) Name() string { return "exit" }

func (e exitExtension) Setup(c *Components) error {
	if e.at == "Setup" {
		c.Lifecycle().Exit(e.code, errors.New("exit in Setup"))
	}
	return nil
}

func (e exitExtension) ExtendMux(m *MuxContext) error {
	switch e.at {
	case "ExtendMux":
		m.Lifecycle.Exit(e.code, nil)
	case "route":
		m.AddRoute(Route{Name: "exit", Pattern: "/exit", Chain: ChainNone, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m.Lifecycle.Exit(e.code, nil)
		})})
	}
	return nil
}

// The code of Lifecycle.Exit, called in New, in Start or once the App runs,
// reaches the process exit of Run.
func TestRunExitCode(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	for _, at := range []string{"Setup", "ExtendMux", "route"} {
		t.Run(at, func(t *testing.T) {
			t.Setenv("CENTRIFUGO_HTTP_SERVER_PORT", "0")
			opts := Options{Extensions: []Extension{exitExtension{at: at, code: 7}}}
			done := make(chan []int, 1)
			built := make(chan *App, 1)
			go func() {
				var codes []int
				run(Centrifugo(opts), writeConfig(t, testConfig), opts, process{
					exit:  func(code int) { codes = append(codes, code) },
					built: func(a *App) { built <- a },
				})
				done <- codes
			}()
			if at == "route" {
				requestExit(t, servingAddr(t, built))
			}
			require.Equal(t, []int{7}, <-done)
		})
	}
}

// Lifecycle.Exit called during Shutdown sets the final code.
func TestLifecycleExitDuringShutdown(t *testing.T) {
	r := &recorder{setup: func(c *Components) error {
		lc := c.Lifecycle()
		lc.OnStop("exit", func(context.Context) error {
			lc.Exit(9, errors.New("failed while stopping"))
			return nil
		})
		return nil
	}}
	a, err := newTestApp(t, testConfig, r)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	code, err := a.Shutdown(ExitStop, 0)
	require.NoError(t, err)
	require.Equal(t, 9, code)
}

// SIGHUP reloads, successful or failing, leave no verifier running.
func TestLifecycleReloadNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	jwks := newTestJWKSServer(t)
	defer jwks.Close()
	cfg := func(endpoint string) string {
		return `{
			"client": {
				"token": {"jwks_public_endpoint": "` + endpoint + `"},
				"subscription_token": {"enabled": true, "jwks_public_endpoint": "` + endpoint + `"}
			},
			"usage_stats": {"disabled": true}
		}`
	}
	r := &recorder{}
	a, err := newTestApp(t, cfg(jwks.URL), r)
	require.NoError(t, err)
	require.NoError(t, a.Start())

	// Successful reload: the replaced verifier parts are closed.
	require.NoError(t, os.WriteFile(a.configFile, []byte(cfg(jwks.URL+"/other")), 0600))
	a.reload()
	require.Contains(t, r.Calls(), "ConfigReloaded")

	// Failing reload after the verifiers were prepared: they are closed.
	require.NoError(t, os.WriteFile(a.configFile, []byte(`{
		"client": {
			"token": {"jwks_public_endpoint": "`+jwks.URL+`/failing"},
			"subscription_token": {"enabled": true, "rsa_public_key": "invalid"}
		},
		"usage_stats": {"disabled": true}
	}`), 0600))
	a.reload()
	calls := r.Calls()
	require.True(t, strings.HasPrefix(calls[len(calls)-1], "ConfigReloaded:"), calls[len(calls)-1])

	a.Close()
}

func newTestJWKSServer(t *testing.T) *testJWKSServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"keys":[]}`)
	})}
	go func() { _ = srv.Serve(ln) }()
	return &testJWKSServer{srv: srv, URL: "http://" + ln.Addr().String()}
}

type testJWKSServer struct {
	srv *http.Server
	URL string
}

func (s *testJWKSServer) Close() {
	_ = s.srv.Close()
}

// servingAddr waits for the App built by run to bind its listeners and
// returns the address of the first.
func servingAddr(t *testing.T, built <-chan *App) string {
	t.Helper()
	var a *App
	select {
	case a = <-built:
	case <-time.After(5 * time.Second):
		require.Fail(t, "App not built")
	}
	require.Eventually(t, func() bool { return len(a.Addrs()) > 0 }, 5*time.Second, 5*time.Millisecond)
	return httpAddr(t, a)
}

// requestExit requests /exit of exitExtension once the App serves on addr.
func requestExit(t *testing.T, addr string) {
	t.Helper()
	c := testHTTPClient(t)
	require.Eventually(t, func() bool {
		resp, err := c.Get("http://" + addr + "/exit")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 5*time.Second, 10*time.Millisecond)
}
