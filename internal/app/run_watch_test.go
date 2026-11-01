package app

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// blockingStop registers a stop step which signals entered and blocks until
// released.
type blockingStop struct {
	logRecorder
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *blockingStop) Setup(c *Components) error {
	c.Lifecycle().OnStop("block", func(context.Context) error {
		e.once.Do(func() { close(e.entered) })
		<-e.release
		return nil
	})
	return nil
}

// configObserver records rejected config changes.
type configObserver struct {
	rejected chan error
}

func (o *configObserver) Name() string                   { return "config observer" }
func (o *configObserver) ConfigReloaded(error)           {}
func (o *configObserver) ConfigChangeRejected(err error) { o.rejected <- err }

// runWatchTest runs the server through run with --watch-config-file and a pid
// file, recording the calls of exit and the Apps built. Signals are handled
// on its signals channel.
type runWatchTest struct {
	configFile string
	pidFile    string
	apps       chan *App
	signals    chan os.Signal
	done       chan struct{}
	built      atomic.Int32

	// app is the App last waited for with next, serving on addr.
	app  *App
	addr string

	mu    sync.Mutex
	codes []int
}

// runWatch writes the config with extra (top-level fields, each preceded by
// a comma) and starts run with p, the HTTP server binding a free port.
func runWatch(t *testing.T, opts Options, p process, extra string) *runWatchTest {
	t.Helper()
	t.Setenv("CENTRIFUGO_HTTP_SERVER_PORT", "0")
	return runWatchConfig(t, opts, p, extra)
}

// runWatchConfig is runWatch without setting the HTTP server port.
func runWatchConfig(t *testing.T, opts Options, p process, extra string) *runWatchTest {
	t.Helper()
	dir := t.TempDir()
	rt := &runWatchTest{
		configFile: filepath.Join(dir, "config.json"),
		pidFile:    filepath.Join(dir, "centrifugo.pid"),
		apps:       make(chan *App, 16),
		signals:    make(chan os.Signal, 4),
		done:       make(chan struct{}),
	}
	rt.write(t, extra)
	cmd := Centrifugo(opts)
	require.NoError(t, cmd.Flags().Set("watch-config-file", "true"))
	p.exit = func(code int) {
		rt.mu.Lock()
		rt.codes = append(rt.codes, code)
		rt.mu.Unlock()
	}
	p.built = func(a *App) {
		rt.built.Add(1)
		rt.apps <- a
	}
	p.signals = rt.signals
	go func() {
		defer close(rt.done)
		run(cmd, rt.configFile, opts, p)
	}()
	return rt
}

// write writes the config file with extra.
func (rt *runWatchTest) write(t *testing.T, extra string) {
	t.Helper()
	writeFile(t, rt.configFile, `{
		"client": {"token": {"hmac_secret_key": "secret"}},
		"pid_file": "`+rt.pidFile+`",
		"usage_stats": {"disabled": true}`+extra+`
	}`)
}

// next waits for the next App built, then until it accepts connections.
func (rt *runWatchTest) next(t *testing.T) *App {
	t.Helper()
	select {
	case rt.app = <-rt.apps:
	case <-time.After(watchWait):
		require.Fail(t, "App not built")
	}
	require.Eventually(t, func() bool { return len(rt.app.Addrs()) > 0 }, watchWait, watchTick)
	rt.addr = httpAddr(t, rt.app)
	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", rt.addr)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, watchWait, watchTick)
	return rt.app
}

// stop sends SIGTERM and waits for run to return.
func (rt *runWatchTest) stop(t *testing.T) {
	t.Helper()
	rt.signals <- syscall.SIGTERM
	rt.wait(t)
}

func (rt *runWatchTest) exitCodes() []int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]int(nil), rt.codes...)
}

func (rt *runWatchTest) wait(t *testing.T) {
	t.Helper()
	select {
	case <-rt.done:
	case <-time.After(watchWait):
		require.Fail(t, "run did not return")
	}
}

func (rt *runWatchTest) pidFileExists() bool {
	_, err := os.Stat(rt.pidFile)
	return err == nil
}

// requirePidFile requires the pid file to hold the PID of the process.
func (rt *runWatchTest) requirePidFile(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(rt.pidFile)
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(os.Getpid()), strings.TrimSpace(string(data)))
}

func requireClosed(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(watchWait):
		require.Fail(t, msg)
	}
}

// freePort returns a port no listener binds at the time of the call.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

func portConfig(port int) string {
	return `, "http_server": {"port": ` + strconv.Itoa(port) + `}`
}

// A valid config change restarts the server in the same process with the
// new config: the new port is served, the old one no longer, the new
// namespace exists. The pid file is kept and holds the same PID.
func TestRunWatchConfigRestart(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	logs := &logRecorder{}
	port1, port2 := freePort(t), freePort(t)
	rt := runWatchConfig(t, Options{Extensions: []Extension{logs}}, process{}, portConfig(port1))
	a := rt.next(t)
	require.Equal(t, strconv.Itoa(port1), strings.Split(rt.addr, ":")[1])
	rt.requirePidFile(t)
	_, _, _, found, err := a.container.ChannelOptions("news:1")
	require.NoError(t, err)
	require.False(t, found)

	rt.write(t, portConfig(port2)+`, "channel": {"namespaces": [{"name": "news"}]}`)
	a = rt.next(t)
	require.Equal(t, strconv.Itoa(port2), strings.Split(rt.addr, ":")[1])
	_, err = net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port1))
	require.Error(t, err)
	_, _, _, found, err = a.container.ChannelOptions("news:1")
	require.NoError(t, err)
	require.True(t, found)
	rt.requirePidFile(t)
	require.Contains(t, logs.String(), "config file changed, restarting")
	require.Empty(t, rt.exitCodes())

	rt.stop(t)
	require.Equal(t, []int{0}, rt.exitCodes())
	require.False(t, rt.pidFileExists())
	require.Equal(t, int32(2), rt.built.Load())
}

// An invalid change is logged and passed to ConfigObservers, and the server
// keeps running; a stop then exits as without --watch-config-file.
func TestRunWatchConfigRejected(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	observer := &configObserver{rejected: make(chan error, 1)}
	logs := &logRecorder{}
	opts := Options{Extensions: []Extension{logs, observer, exitExtension{at: "route", code: 0}}}
	rt := runWatch(t, opts, process{}, "")
	rt.next(t)
	writeFile(t, rt.configFile, `{"client": {"insecure": tr`)
	select {
	case err := <-observer.rejected:
		require.ErrorContains(t, err, "error reading config file")
	case <-time.After(watchWait):
		require.Fail(t, "change not rejected")
	}
	require.Contains(t, logs.String(), "config file change rejected, keep running with current config")
	requestExit(t, rt.addr)
	rt.wait(t)
	require.Equal(t, []int{0}, rt.exitCodes())
	require.Equal(t, int32(1), rt.built.Load())
	require.False(t, rt.pidFileExists())
}

// The backstop of a restart whose shutdown can't return exits 1 and removes
// the pid file; no App is built after it.
func TestRunWatchConfigBackstopRestart(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	ext := &blockingStop{entered: make(chan struct{}), release: make(chan struct{})}
	rt := runWatch(t, Options{Extensions: []Extension{ext}}, process{backstop: 100 * time.Millisecond}, "")
	rt.next(t)
	rt.write(t, `, "client": {"insecure": true}`)
	requireClosed(t, ext.entered, "shutdown did not reach the stop step")
	require.Eventually(t, func() bool { return len(rt.exitCodes()) == 1 }, watchWait, watchTick)
	logs := ext.String()
	require.Contains(t, logs, "shutdown timeout reached")
	require.Contains(t, logs, `"step":"block"`)
	require.Equal(t, []int{1}, rt.exitCodes())
	require.False(t, rt.pidFileExists())
	close(ext.release)
	rt.wait(t)
	require.Equal(t, []int{1}, rt.exitCodes())
	require.Equal(t, int32(1), rt.built.Load())
}

// A restart whose shutdown returned after shutdown.timeout passed exits 1
// and removes the pid file: what was left running stays in the process.
func TestRunWatchConfigShutdownTimeoutRestart(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	ext := &blockingStop{entered: make(chan struct{}), release: make(chan struct{})}
	rt := runWatch(t, Options{Extensions: []Extension{ext}}, process{}, `, "shutdown": {"timeout": "500ms"}`)
	rt.next(t)
	rt.write(t, `, "shutdown": {"timeout": "500ms"}, "client": {"insecure": true}`)
	requireClosed(t, ext.entered, "shutdown did not reach the stop step")
	rt.wait(t)
	close(ext.release)
	require.Equal(t, []int{1}, rt.exitCodes())
	require.False(t, rt.pidFileExists())
	require.Equal(t, int32(1), rt.built.Load())
	require.Contains(t, ext.String(), "shutdown step did not finish in time, leaving it running")
}

// failSecondSetup fails Setup of every App after the first one.
type failSecondSetup struct {
	calls atomic.Int32
}

func (e *failSecondSetup) Name() string { return "fail second setup" }

func (e *failSecondSetup) Setup(*Components) error {
	if e.calls.Add(1) > 1 {
		return errors.New("setup failed")
	}
	return nil
}

// A changed config passing the checks with which the restarted App fails to
// build or start exits 1 and removes the pid file, as a failed start does.
func TestRunWatchConfigRestartFails(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		defer goleak.VerifyNone(t, leakOptions()...)

		rt := runWatch(t, Options{Extensions: []Extension{&failSecondSetup{}}}, process{}, "")
		rt.next(t)
		rt.write(t, `, "client": {"insecure": true}`)
		rt.wait(t)
		require.Equal(t, []int{1}, rt.exitCodes())
		require.False(t, rt.pidFileExists())
	})
	t.Run("Start", func(t *testing.T) {
		defer goleak.VerifyNone(t, leakOptions()...)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer func() { _ = ln.Close() }()
		busy := ln.Addr().(*net.TCPAddr).Port

		rt := runWatchConfig(t, Options{}, process{}, portConfig(freePort(t)))
		rt.next(t)
		rt.write(t, `, "http_server": {"address": "127.0.0.1", "port": `+strconv.Itoa(busy)+`}`)
		rt.wait(t)
		require.Equal(t, []int{1}, rt.exitCodes())
		require.False(t, rt.pidFileExists())
		require.Equal(t, int32(2), rt.built.Load())
	})
}

// A restart begins from the environment the process started with and loads
// .env again: a changed .env applies, a variable removed from it is gone.
func TestRunWatchConfigDotEnv(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	for _, key := range []string{"CENTRIFUGO_HTTP_API_KEY", "CENTRIFUGO_HTTP_API_INSECURE"} {
		t.Setenv(key, "") // Restored after the test.
		require.NoError(t, os.Unsetenv(key))
	}
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, ".env"), "CENTRIFUGO_HTTP_API_KEY=key1\nCENTRIFUGO_HTTP_API_INSECURE=true\n")
	rt := runWatch(t, Options{}, process{}, "")
	a := rt.next(t)
	require.Equal(t, "key1", a.cfg.HttpAPI.Key)
	require.True(t, a.cfg.HttpAPI.Insecure)

	writeFile(t, filepath.Join(dir, ".env"), "CENTRIFUGO_HTTP_API_KEY=key2\n")
	rt.write(t, `, "client": {"insecure": true}`)
	a = rt.next(t)
	require.Equal(t, "key2", a.cfg.HttpAPI.Key)
	require.False(t, a.cfg.HttpAPI.Insecure)
	rt.stop(t)
	require.Equal(t, []int{0}, rt.exitCodes())
}

func TestRestoreEnviron(t *testing.T) {
	t.Setenv("CENTRIFUGO_TEST_KEPT", "a=b")
	t.Setenv("CENTRIFUGO_TEST_CHANGED", "old")
	t.Setenv("CENTRIFUGO_TEST_REMOVED", "")
	require.NoError(t, os.Unsetenv("CENTRIFUGO_TEST_REMOVED"))
	environ := os.Environ()
	require.NoError(t, os.Setenv("CENTRIFUGO_TEST_CHANGED", "new"))
	require.NoError(t, os.Setenv("CENTRIFUGO_TEST_REMOVED", "x"))
	restoreEnviron(environ)
	require.Equal(t, "a=b", os.Getenv("CENTRIFUGO_TEST_KEPT"))
	require.Equal(t, "old", os.Getenv("CENTRIFUGO_TEST_CHANGED"))
	_, ok := os.LookupEnv("CENTRIFUGO_TEST_REMOVED")
	require.False(t, ok)
	got := os.Environ()
	slices.Sort(got)
	slices.Sort(environ)
	require.Equal(t, environ, got)
}

func TestSplitEnv(t *testing.T) {
	for _, tc := range []struct{ kv, key, value string }{
		{"A=b", "A", "b"},
		{"A=b=c", "A", "b=c"},
		{"A=", "A", ""},
		{"=C:=C:\\dir", "=C:", "C:\\dir"},
	} {
		key, value := splitEnv(tc.kv)
		require.Equal(t, tc.key, key, tc.kv)
		require.Equal(t, tc.value, value, tc.kv)
	}
}

// Restarts in one process with real traffic leave nothing running once the
// last App stopped; the metrics of each App are its own and keep updating.
func TestRunWatchConfigRestartsNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	setGRPCPortsFree(t)
	cfg := strings.TrimSuffix(strings.TrimPrefix(leakTestConfig, "{"), "}")
	rt := runWatch(t, Options{}, process{}, ","+cfg)
	for i := 0; i < 4; i++ {
		a := rt.next(t)
		serveTraffic(t, a)
		if i < 3 {
			rt.write(t, ","+cfg+`, "channel": {"namespaces": [{"name": "ns`+strconv.Itoa(i)+`"}]}`)
		}
	}
	rt.stop(t)
	require.Equal(t, []int{0}, rt.exitCodes())
	require.Equal(t, int32(4), rt.built.Load())
	require.False(t, rt.pidFileExists())
}

// metricsText returns the /metrics output of the App serving on addr.
func metricsText(t *testing.T, addr string) string {
	t.Helper()
	resp, err := testHTTPClient(t).Get("http://" + addr + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

// metricFamilies returns the names of the metric families in /metrics text.
func metricFamilies(text string) []string {
	var names []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		if f := strings.Fields(scanner.Text()); len(f) >= 3 && f[0] == "#" && f[1] == "TYPE" {
			names = append(names, f[2])
		}
	}
	return names
}

// The registry of an App has the collectors of the default registry, and
// its /metrics exposes the metrics of the handler as the default handler
// does.
func TestMetricsRegistryCollectors(t *testing.T) {
	gathered := func(g prometheus.Gatherer) []string {
		families, err := g.Gather()
		require.NoError(t, err)
		var names []string
		for _, f := range families {
			names = append(names, f.GetName())
		}
		return names
	}
	require.Equal(t, gathered(prometheus.DefaultGatherer), gathered(newMetricsRegistry()))

	a, err := newTestApp(t, `{"prometheus": {"enabled": true}, "usage_stats": {"disabled": true}}`)
	require.NoError(t, err)
	require.NoError(t, a.Start())
	addr := httpAddr(t, a)
	_ = metricsText(t, addr)
	families := metricFamilies(metricsText(t, addr))
	for _, name := range []string{
		"go_goroutines", "process_cpu_seconds_total",
		"promhttp_metric_handler_requests_total", "promhttp_metric_handler_requests_in_flight",
		"centrifugo_node_build",
	} {
		require.Contains(t, families, name)
	}
}

// A restart applies a changed metrics config: with native histograms the
// API command histogram has only its +Inf bucket in text format and its summary is
// gone. Metrics keep updating after the restart.
func TestRunWatchConfigRestartMetricsConfig(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	base := `, "http_api": {"key": "api-key"}, "uni_http_stream": {"enabled": true}, "prometheus": {"enabled": true`
	rt := runWatch(t, Options{}, process{}, base+`}`)
	a := rt.next(t)
	serveTraffic(t, a)
	text := metricsText(t, rt.addr)
	require.Greater(t, strings.Count(text, "centrifugo_api_command_duration_seconds_histogram_bucket{"), 1)
	require.Contains(t, text, "centrifugo_api_command_duration_seconds{")

	rt.write(t, base+`, "native_histograms": true}`)
	a = rt.next(t)
	serveTraffic(t, a)
	text = metricsText(t, rt.addr)
	require.Contains(t, text, "centrifugo_api_command_duration_seconds_histogram_count{")
	require.Equal(t, 1, strings.Count(text, "centrifugo_api_command_duration_seconds_histogram_bucket{"))
	require.NotContains(t, text, "centrifugo_api_command_duration_seconds{")
	rt.stop(t)
	require.Equal(t, []int{0}, rt.exitCodes())
}

// closeBlocker has a log writer whose closer, run by Close, signals entered
// and blocks until released, for the first App only.
type closeBlocker struct {
	logRecorder
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (e *closeBlocker) LogWriters(config.Config) ([]io.Writer, func(), error) {
	first := e.calls.Add(1) == 1
	return []io.Writer{&e.logRecorder}, func() {
		if first {
			close(e.entered)
			<-e.release
		}
	}, nil
}

// setupBlocker blocks in Setup of the second App, during New, until
// released.
type setupBlocker struct {
	logRecorder
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (e *setupBlocker) Setup(*Components) error {
	if e.calls.Add(1) == 2 {
		close(e.entered)
		<-e.release
	}
	return nil
}

// startBlocker blocks in ExtendMux of the second App, during Start, until
// shutdown begins.
type startBlocker struct {
	logRecorder
	entered chan struct{}
	calls   atomic.Int32
}

func (e *startBlocker) ExtendMux(m *MuxContext) error {
	if e.calls.Add(1) == 2 {
		close(e.entered)
		<-m.Lifecycle.Context().Done()
	}
	return nil
}
