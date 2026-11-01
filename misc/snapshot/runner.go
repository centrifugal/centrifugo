package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
)

// Options configure a snapshot run.
type Options struct {
	Binary      string
	Configs     []string // base config templates, merged in order.
	Out         string
	With        map[string]bool // enabled integration services.
	TmpRoot     string
	Only        *regexp.Regexp
	Timeout     time.Duration
	RedisAddr   string
	PostgresDSN string
	KafkaAddr   string
	NatsURL     string
}

// defaultReloadOutcome matches log messages that end a SIGHUP reload.
var defaultReloadOutcome = []string{
	`configuration successfully reloaded`, `error reloading`, `error validating config`, `error reading config`,
}

// run is one scenario (variant) execution.
type run struct {
	opts     Options
	sc       *Scenario
	id       string // output dir name.
	tmp      string
	norm     *Normalizer
	td       TemplateData
	cfg      any // current config (decoded JSON).
	cfgPath  string
	env      []string
	proc     *process
	rec      *Recorder
	clients  map[string]conn
	out      []string // transcript lines.
	metrics  []string
	files    map[string]string // extra output files.
	sigIndex int               // first log line after the final signal, -1 if none.
	// extraScrapes counts metric scrapes beyond one per metrics op, by
	// path; their request log lines are dropped so the log is stable.
	extraScrapes map[string]int
	exitDone     bool
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// runScenario executes one scenario variant and writes its output directory.
func runScenario(opts Options, raw []byte, name, variant string, v Variant) error {
	id := name
	if variant != "" {
		id = name + "@" + variant
	}
	tmp, err := os.MkdirTemp(opts.TmpRoot, "snap-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if real, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = real
	}
	r := &run{opts: opts, id: id, tmp: tmp, norm: NewNormalizer(), clients: map[string]conn{}, files: map[string]string{}, sigIndex: -1, extraScrapes: map[string]int{}}

	r.rec, err = NewRecorder()
	if err != nil {
		return err
	}
	defer r.rec.Close()

	ports := map[string]int{}
	for _, p := range []string{"http", "internal", "grpc_api", "uni_grpc"} {
		ports[p], err = freePort()
		if err != nil {
			return err
		}
	}
	ports["proxy_http"] = r.rec.HTTPPort()
	ports["proxy_grpc"] = r.rec.GRPCPort()
	unique := fmt.Sprintf("snap%d%d", time.Now().UnixNano()%1e9, os.Getpid())
	r.td = TemplateData{
		Port:        ports,
		ProxyHTTP:   fmt.Sprintf("http://127.0.0.1:%d", ports["proxy_http"]),
		ProxyGRPC:   fmt.Sprintf("grpc://127.0.0.1:%d", ports["proxy_grpc"]),
		TmpDir:      tmp,
		Unique:      unique,
		RedisAddr:   opts.RedisAddr,
		PostgresDSN: opts.PostgresDSN,
		KafkaAddr:   opts.KafkaAddr,
		NatsURL:     opts.NatsURL,
	}
	// Placeholders: longer and more specific first (AddLiteral sorts by length).
	r.norm.AddLiteral(tmp, "<tmp>")
	r.norm.AddLiteral(unique, "<unique>")
	if h, err := os.Hostname(); err == nil {
		// Default node name is hostname_port.
		r.norm.AddLiteral(h+"_"+strconv.Itoa(ports["http"]), "<node-name>")
	}
	for pname, p := range ports {
		ps := strconv.Itoa(p)
		r.norm.AddLiteral("127.0.0.1:"+ps, "127.0.0.1:<port:"+pname+">")
		r.norm.AddLiteral("localhost:"+ps, "localhost:<port:"+pname+">")
		r.norm.AddLiteral("[::]:"+ps, "[::]:<port:"+pname+">")
		r.norm.AddLiteral(":"+ps, ":<port:"+pname+">")
		r.norm.AddLiteral(`"`+ps+`"`, `"<port:`+pname+`>"`)
	}

	sc, err := parseScenario(name, raw, r.td)
	if err != nil {
		return err
	}
	r.sc = sc
	if variant != "" {
		v = sc.Variants[variant] // templates rendered for this run.
	}

	// Config: ports, then base templates, scenario patch, variant patch.
	// A scenario may unset internal_port to serve everything on one port.
	var cfg any
	portPatch := fmt.Sprintf(`{"http_server":{"address":"127.0.0.1","port":%d,"internal_port":"%d","internal_address":"127.0.0.1"},"grpc_api":{"port":%d,"address":"127.0.0.1"},"uni_grpc":{"port":%d,"address":"127.0.0.1"}}`,
		ports["http"], ports["internal"], ports["grpc_api"], ports["uni_grpc"])
	if cfg, err = mergeRaw(map[string]any{}, json.RawMessage(portPatch)); err != nil {
		return err
	}
	for _, c := range opts.Configs {
		data, err := os.ReadFile(c)
		if err != nil {
			return err
		}
		text, err := renderTemplate(c, string(data), r.td)
		if err != nil {
			return err
		}
		if cfg, err = mergeRaw(cfg, json.RawMessage(text)); err != nil {
			return fmt.Errorf("config %s: %w", c, err)
		}
	}
	if cfg, err = mergeRaw(cfg, sc.Config); err != nil {
		return fmt.Errorf("scenario config: %w", err)
	}
	if cfg, err = mergeRaw(cfg, v.Config); err != nil {
		return fmt.Errorf("variant config: %w", err)
	}
	r.cfg = cfg
	r.cfgPath = filepath.Join(tmp, "config.json")
	if err := r.writeConfig(); err != nil {
		return err
	}

	r.env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + tmp, "TMPDIR=" + tmp}
	for _, m := range []map[string]string{sc.Env, v.Env} {
		keys := sortedKeys(m)
		for _, k := range keys {
			r.env = append(r.env, k+"="+m[k])
		}
	}
	r.rec.SetRules(sc.Proxy)

	if sc.Description != "" {
		r.line("# " + sc.Description)
	}
	for i, st := range sc.Setup {
		st.Comment = strings.TrimSpace("setup " + st.Comment)
		if err := r.step(i+1, st); err != nil {
			r.line(fmt.Sprintf("! step error: %v", err))
		}
	}
	if sc.runsServer() {
		if err := r.startServer(); err != nil {
			return err
		}
	}
	for i, st := range sc.Steps {
		if r.proc != nil && r.proc.isExited() && !r.exitDone {
			r.line(fmt.Sprintf("! server exited, %d steps not run", len(sc.Steps)-i))
			break
		}
		if err := r.step(i+1, st); err != nil {
			r.line(fmt.Sprintf("! step error: %v", err))
		}
		r.flushProxy()
	}
	r.finish()
	for i, st := range sc.Teardown {
		st.Comment = strings.TrimSpace("teardown " + st.Comment)
		if err := r.step(i+1, st); err != nil {
			r.line(fmt.Sprintf("! step error: %v", err))
		}
	}
	return r.write()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (r *run) writeConfig() error {
	data, err := json.MarshalIndent(r.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.cfgPath, data, 0o600)
}

func (r *run) line(s string) { r.out = append(r.out, s) }

// event records a JSON value under a prefix.
func (r *run) event(prefix string, v any) {
	r.line(prefix + " " + CanonicalJSON(r.norm.Value(v)))
}

func (r *run) startServer() error {
	var err error
	r.proc, err = startProcess(r.opts.Binary, []string{"--config", r.cfgPath}, r.tmp, r.env)
	if err != nil {
		return err
	}
	// Ready when the listeners the server announces ("serving ... on
	// <addr>") accept connections. The HTTP servers are announced together,
	// so after the first announcement the output is given a moment to settle.
	deadline := time.Now().Add(15 * time.Second)
	idx, _ := r.proc.waitLine(0, []*regexp.Regexp{servingRe}, 15*time.Second)
	if idx < 0 {
		if r.proc.isExited() {
			r.line("startup: process exited before ready")
		} else {
			r.line("startup: timeout waiting for serving announcement")
		}
		return nil
	}
	r.settleLogsFor(100 * time.Millisecond)
	var addrs []string
	for _, l := range r.proc.linesFrom(0) {
		if m := servingRe.FindStringSubmatch(logMessage(l)); m != nil {
			addrs = append(addrs, m[1])
		}
	}
	for _, a := range addrs {
		for {
			if r.proc.isExited() {
				r.line("startup: process exited before ready")
				return nil
			}
			c, err := net.DialTimeout("tcp", a, 200*time.Millisecond)
			if err == nil {
				_ = c.Close()
				break
			}
			if time.Now().After(deadline) {
				r.line("startup: timeout waiting for " + r.norm.String(a))
				return nil
			}
			time.Sleep(20 * time.Millisecond) //nolint:forbidigo // polling a port.
		}
	}
	r.line("startup: ready")
	return nil
}

var servingRe = regexp.MustCompile(`^serving .* on (\S+)`)

func (r *run) internalPort() string {
	if p, _ := lookupPath(r.cfg, "http_server.internal_port"); p == "" || p == nil {
		return "http"
	}
	return "internal"
}

func (r *run) addr(name string) string {
	return fmt.Sprintf("127.0.0.1:%d", r.td.Port[name])
}

func (r *run) baseURL(port string) string {
	switch port {
	case "", "external":
		port = "http"
	case "internal":
		port = r.internalPort()
	}
	return "http://" + r.addr(port)
}

func (r *run) flushProxy() {
	r.printProxy(r.rec.Take(false))
}

func (r *run) printProxy(reqs []ProxyRequest) {
	lines := make([]string, 0, len(reqs))
	for _, req := range reqs {
		lines = append(lines, "proxy "+CanonicalJSON(r.norm.Value(toJSONValue(req))))
	}
	sortProxyRequests(lines)
	r.out = append(r.out, lines...)
}

// toJSONValue converts a struct into decoded-JSON form for normalization.
func toJSONValue(v any) any {
	b, _ := json.Marshal(v)
	out, _ := DecodeJSON(b)
	return out
}

func (r *run) serverStep() error {
	if r.proc == nil {
		return fmt.Errorf("no server")
	}
	if r.proc.isExited() {
		return fmt.Errorf("server exited")
	}
	return nil
}

func (r *run) step(n int, st Step) error {
	header := fmt.Sprintf("## %d %s", n, st.Op)
	if st.Client != "" {
		header += " " + st.Client
	}
	if st.Comment != "" {
		header += " -- " + st.Comment
	}
	r.line(header)
	timeout := r.opts.Timeout
	switch st.Op {
	case "connect":
		if err := r.serverStep(); err != nil {
			return err
		}
		path := "/connection/websocket"
		if st.Path != "" {
			path = st.Path
		}
		if st.Query != "" {
			path += "?" + st.Query
		}
		c, resp, err := dialWS(wsURL(r.baseURL(st.Port), path), st.Headers)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err != nil {
			code := 0
			if resp != nil {
				code = resp.StatusCode
			}
			r.line(fmt.Sprintf("dial failed: status=%d", code))
			return nil
		}
		r.clients[st.Client] = c
		r.line("dial ok")
	case "uni":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.uniConnect(st)
	case "send":
		c, _ := r.clients[st.Client].(*wsClient)
		if c == nil {
			return fmt.Errorf("unknown bidirectional client %q", st.Client)
		}
		data, err := expandJSON(st.Cmd)
		if err != nil {
			return err
		}
		v, _ := DecodeJSON(data)
		cmd, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("cmd must be an object")
		}
		id, err := c.send(cmd, !st.NoReply)
		if err != nil {
			return err
		}
		r.event("> "+st.Client, cmd)
		if st.NoReply {
			return nil
		}
		reply, closed := c.waitReply(id, timeout)
		if closed != nil {
			r.recordPushes(st.Client, c.takePushes(), false)
			r.event("closed "+st.Client, toJSONValue(closed))
			delete(r.clients, st.Client)
			return nil
		}
		r.event("< "+st.Client, reply)
	case "pushes":
		c := r.clients[st.Client]
		if c == nil {
			return fmt.Errorf("unknown client %q", st.Client)
		}
		pushes, err := c.waitPushes(st.Count, timeout)
		r.recordPushes(st.Client, pushes, st.Sort)
		return err
	case "wait_close":
		c := r.clients[st.Client]
		if c == nil {
			return fmt.Errorf("unknown client %q", st.Client)
		}
		ci := c.waitClose(timeout)
		r.recordPushes(st.Client, c.takePushes(), false)
		r.event("closed "+st.Client, toJSONValue(ci))
		delete(r.clients, st.Client)
	case "close":
		c := r.clients[st.Client]
		if c == nil {
			return fmt.Errorf("unknown client %q", st.Client)
		}
		r.recordPushes(st.Client, c.takePushes(), false)
		c.close()
		delete(r.clients, st.Client)
	case "api":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.httpAPI(st)
	case "grpc_api":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.grpcAPI(st)
	case "http":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.httpRequest(st)
	case "routes":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.routes(st)
	case "metrics":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.scrapeMetrics(n, st)
	case "wait_proxy":
		ok := r.rec.WaitKind(st.Kind, max(st.Count, 1), timeout)
		if !ok {
			r.line(fmt.Sprintf("timeout waiting for %d %s proxy requests", max(st.Count, 1), st.Kind))
		}
		r.printProxy(r.rec.TakeKind(st.Kind, max(st.Count, 1)))
	case "proxy_rules":
		r.rec.SetRules(st.Rules)
	case "wait_log":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.waitLog(0, st.Match)
	case "reload":
		if err := r.serverStep(); err != nil {
			return err
		}
		return r.reload(st)
	case "sql":
		return r.sql(st)
	case "exec":
		return r.exec(n, st)
	case "shutdown":
		if err := r.serverStep(); err != nil {
			return err
		}
		r.shutdown(st.Signal)
	default:
		return fmt.Errorf("unknown op %q", st.Op)
	}
	return nil
}

func (r *run) recordPushes(client string, pushes []any, sorted bool) {
	lines := make([]string, 0, len(pushes))
	for _, p := range pushes {
		lines = append(lines, "push "+client+" "+CanonicalJSON(r.norm.Value(p)))
	}
	if sorted {
		sort.Strings(lines)
	}
	r.out = append(r.out, lines...)
}

func (r *run) waitLog(from int, patterns []string) error {
	var res []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return err
		}
		res = append(res, re)
	}
	idx, l := r.proc.waitLine(from, res, r.opts.Timeout)
	if idx < 0 {
		r.line("log: timeout waiting for " + strings.Join(patterns, " | "))
		return nil
	}
	r.line("log " + r.norm.normalizeLogLine(l))
	return nil
}

func (r *run) reload(st Step) error {
	if st.RawConfig != nil {
		if err := os.WriteFile(r.cfgPath, []byte(*st.RawConfig), 0o600); err != nil {
			return err
		}
	} else {
		var err error
		if r.cfg, err = mergeRaw(r.cfg, st.Config); err != nil {
			return err
		}
		if err := r.writeConfig(); err != nil {
			return err
		}
	}
	from := r.proc.numLines()
	if err := r.proc.sendSignal(syscall.SIGHUP); err != nil {
		return err
	}
	patterns := st.Match
	if len(patterns) == 0 {
		patterns = defaultReloadOutcome
	}
	return r.waitLog(from, patterns)
}

func (r *run) sql(st Step) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.Timeout)
	defer cancel()
	dsn := st.DSN
	if dsn == "" {
		dsn = r.opts.PostgresDSN
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	_, err = conn.Exec(ctx, st.Exec)
	if err != nil {
		r.line("sql error: " + r.norm.String(err.Error()))
		return nil
	}
	r.line("sql ok")
	return nil
}

func (r *run) exec(n int, st Step) error {
	dir, err := os.MkdirTemp(r.tmp, "exec-")
	if err != nil {
		return err
	}
	r.norm.AddLiteral(dir, "<execdir>")
	for name, content := range st.Files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.opts.Binary, st.Args...)
	cmd.Dir = dir
	cmd.Env = r.env
	cmd.Stdin = strings.NewReader(st.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return err
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "$ centrifugo %s\nexit code: %d\n", r.norm.String(strings.Join(st.Args, " ")), code)
	b.WriteString("--- stdout\n")
	b.WriteString(r.normalizeExecOutput(stdout.String(), st))
	b.WriteString("--- stderr\n")
	b.WriteString(r.normalizeExecOutput(stderr.String(), st))
	for _, f := range st.ReadFiles {
		data, err := os.ReadFile(filepath.Join(dir, f))
		fmt.Fprintf(&b, "--- file %s\n", f)
		if err != nil {
			b.WriteString("not found\n")
			continue
		}
		b.WriteString(r.normalizeExecOutput(string(data), st))
	}
	name := fmt.Sprintf("exec-%02d", n)
	if len(st.Args) > 0 {
		name += "-" + regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.Join(st.Args, "_"), "_")
	}
	if len(name) > 80 {
		name = name[:80]
	}
	r.files[name+".txt"] = b.String()
	r.line(fmt.Sprintf("exit code %d, output in %s.txt", code, name))
	return nil
}

var goVersionRe = regexp.MustCompile(`go1\.\d+(\.\d+)?`)

// normalizeExecOutput keeps CLI output as is (durations there are config
// values), normalizing JSON log lines and run-specific strings only.
func (r *run) normalizeExecOutput(s string, st Step) string {
	for _, k := range sortedKeys(st.Replace) {
		s = regexp.MustCompile(k).ReplaceAllString(s, st.Replace[k])
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, `{"level":`) {
			lines[i] = r.norm.normalizeLogLine(l)
			continue
		}
		l = r.norm.StringNoDurations(l)
		l = goVersionRe.ReplaceAllString(l, "<go-version>")
		lines[i] = l
	}
	out := strings.Join(lines, "\n")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n[no newline at end]\n"
	}
	return out
}

func (r *run) httpClient() *http.Client {
	return &http.Client{
		Timeout: r.opts.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (r *run) apiKey(field string) string {
	k, _ := lookupPath(r.cfg, field)
	s, _ := k.(string)
	return s
}

func (r *run) httpAPI(st Step) error {
	var body []byte
	if st.RawBody != nil {
		body = []byte(*st.RawBody)
	} else {
		var err error
		body, err = expandJSON(st.Body)
		if err != nil {
			return err
		}
	}
	path := st.Path
	if path == "" {
		path = "/api/" + st.Method
	}
	req, err := http.NewRequest(http.MethodPost, r.baseURL(orDefault(st.Port, "internal"))+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	switch st.Auth {
	case "", "key":
		req.Header.Set("X-API-Key", r.apiKey("http_api.key"))
	case "bad":
		req.Header.Set("X-API-Key", "bad-key")
	case "none":
	default:
		req.Header.Set("Authorization", st.Auth)
	}
	for k, v := range st.Headers {
		req.Header.Set(k, v)
	}
	r.line("api " + path + " " + r.norm.NormalizeJSONBytes(body))
	resp, err := r.httpClient().Do(req)
	if err != nil {
		r.line("api error: " + r.norm.String(err.Error()))
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	r.line(fmt.Sprintf("api status %d content-type=%q", resp.StatusCode, resp.Header.Get("Content-Type")))
	r.line("api resp " + r.norm.NormalizeJSONBytes(data))
	return nil
}

func (r *run) httpRequest(st Step) error {
	var body io.Reader
	if st.RawBody != nil {
		body = strings.NewReader(*st.RawBody)
	} else if len(st.Body) > 0 {
		b, err := expandJSON(st.Body)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	method := orDefault(st.Method, http.MethodGet)
	req, err := http.NewRequest(method, r.baseURL(st.Port)+st.Path, body)
	if err != nil {
		return err
	}
	for k, v := range st.Headers {
		req.Header.Set(k, v)
	}
	resp, err := r.httpClient().Do(req)
	if err != nil {
		r.line("http error: " + r.norm.String(err.Error()))
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	r.line(fmt.Sprintf("http %s %s %s -> %d content-type=%q", orDefault(st.Port, "external"), method, st.Path, resp.StatusCode, resp.Header.Get("Content-Type")))
	if len(data) > 0 {
		for _, k := range sortedKeys(st.Replace) {
			data = regexp.MustCompile(k).ReplaceAll(data, []byte(st.Replace[k]))
		}
		r.line("http body " + r.norm.NormalizeJSONBytes(data))
	}
	return nil
}

// defaultRoutePaths is probed on each port by the "routes" op without paths.
var defaultRoutePaths = []string{
	"/", "/connection/websocket", "/connection/http_stream", "/connection/sse",
	"/connection/uni_websocket", "/connection/uni_sse", "/connection/uni_http_stream",
	"/connection/webtransport", "/emulation", "/api", "/api/publish", "/api/info", "/api/batch",
	"/api/unknown_method", "/metrics", "/health", "/swagger", "/swagger/", "/swagger/index.html",
	"/debug/pprof/", "/debug/pprof/cmdline", "/admin", "/admin/", "/admin/init", "/admin/auth", "/admin/api",
	"/connection/init", "/nonexistent",
}

func (r *run) routes(st Step) error {
	paths := st.Paths
	if len(paths) == 0 {
		paths = defaultRoutePaths
	}
	methods := st.Methods
	if len(methods) == 0 {
		methods = []string{http.MethodGet, http.MethodPost}
	}
	ports := []string{"external", "internal"}
	if st.Port != "" {
		ports = []string{st.Port}
	}
	for _, port := range ports {
		for _, p := range paths {
			for _, m := range methods {
				req, err := http.NewRequest(m, r.baseURL(port)+p, nil)
				if err != nil {
					return err
				}
				for k, v := range st.Headers {
					req.Header.Set(k, v)
				}
				resp, err := r.httpClient().Do(req)
				if err != nil {
					r.line(fmt.Sprintf("route %s %s %s -> error", port, m, p))
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				ct := resp.Header.Get("Content-Type")
				if i := strings.IndexByte(ct, ';'); i >= 0 {
					ct = ct[:i]
				}
				loc := ""
				if l := resp.Header.Get("Location"); l != "" {
					loc = " location=" + l
				}
				for _, h := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"} {
					if v := resp.Header.Get(h); v != "" {
						loc += " " + h + "=" + v
					}
				}
				r.line(fmt.Sprintf("route %s %s %s -> %d %s%s", port, m, p, resp.StatusCode, ct, loc))
			}
		}
	}
	return nil
}

func (r *run) scrapeMetrics(n int, st Step) error {
	path := orDefault(st.Path, "/metrics")
	var filter *regexp.Regexp
	if st.Filter != "" {
		var err error
		filter, err = regexp.Compile(st.Filter)
		if err != nil {
			return err
		}
	}
	// Counters and gauges settle after asynchronous work (disconnect
	// cleanup, broker deliveries): scrape until two scrapes in a row agree.
	// Series about the scrape path itself change with every scrape, so
	// their values are dropped.
	pathLabel := regexp.MustCompile(`path="` + regexp.QuoteMeta(path) + `"[^ ]* .*$`)
	var prev []string
	status := 0
	deadline := time.Now().Add(r.opts.Timeout)
	scrapes := 0
	for {
		code, lines, err := r.scrapeOnce(path, filter)
		if err != nil {
			r.line("metrics error: " + r.norm.String(err.Error()))
			return nil
		}
		scrapes++
		status = code
		for i, l := range lines {
			if pathLabel.MatchString(l) {
				lines[i] = l[:strings.LastIndexByte(l, ' ')] + " <v>"
			}
		}
		if prev != nil && slices.Equal(prev, lines) {
			break
		}
		prev = lines
		if time.Now().After(deadline) {
			r.line("metrics did not settle")
			break
		}
		time.Sleep(50 * time.Millisecond) //nolint:forbidigo // polling.
	}
	r.extraScrapes[path] += scrapes - 1
	r.metrics = append(r.metrics, fmt.Sprintf("## step %d status %d", n, status))
	r.metrics = append(r.metrics, prev...)
	r.line(fmt.Sprintf("metrics status %d, %d lines in metrics.txt", status, len(prev)))
	return nil
}

func (r *run) scrapeOnce(path string, filter *regexp.Regexp) (int, []string, error) {
	req, err := http.NewRequest(http.MethodGet, r.baseURL("internal")+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "text/plain;version=0.0.4")
	resp, err := r.httpClient().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	// Label values may carry IDs first seen here: no new UUID numbers.
	saved := r.norm.frozen
	r.norm.frozen = true
	lines := normalizeMetrics(string(data), r.norm, filter)
	r.norm.frozen = saved
	return resp.StatusCode, lines, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// shutdown signals the server, records its exit and the clients' closes.
func (r *run) shutdown(sig string) {
	s := syscall.SIGTERM
	switch sig {
	case "SIGINT":
		s = syscall.SIGINT
	case "SIGKILL":
		s = syscall.SIGKILL
	}
	// Clients first record pushes they got before the signal.
	for _, name := range r.clientNames() {
		r.recordPushes(name, r.clients[name].takePushes(), false)
	}
	r.settleLogs()
	r.sigIndex = r.proc.numLines()
	_ = r.proc.sendSignal(s)
	code, killed := r.proc.wait(30 * time.Second)
	r.exitDone = true
	if killed {
		r.line("exit: killed after timeout")
	} else {
		r.line(fmt.Sprintf("exit code %d", code))
	}
	for _, name := range r.clientNames() {
		c := r.clients[name]
		ci := c.waitClose(5 * time.Second)
		// Joins, leaves and publications sent while the node disconnects
		// clients race with the disconnection: keep disconnect pushes only.
		var pushes []any
		for _, p := range c.takePushes() {
			if m, ok := p.(map[string]any); ok {
				if inner, ok := m["push"].(map[string]any); ok {
					m = inner
				}
				if _, ok := m["disconnect"]; ok {
					pushes = append(pushes, p)
				}
			}
		}
		r.recordPushes(name, pushes, false)
		r.event("closed "+name, toJSONValue(ci))
		delete(r.clients, name)
	}
}

// settleLogs waits until the server logged nothing for a while, so that
// lines caused by earlier steps are not counted as shutdown lines.
func (r *run) settleLogs() {
	r.settleLogsFor(200 * time.Millisecond)
}

func (r *run) settleLogsFor(quiet time.Duration) {
	deadline := time.Now().Add(3 * time.Second)
	n := r.proc.numLines()
	for time.Now().Before(deadline) {
		time.Sleep(quiet) //nolint:forbidigo // waiting for quiet output.
		m := r.proc.numLines()
		if m == n {
			return
		}
		n = m
	}
}

func (r *run) clientNames() []string {
	names := make([]string, 0, len(r.clients))
	for k := range r.clients {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (r *run) finish() {
	if r.proc != nil && !r.exitDone {
		r.line("## end shutdown")
		if r.proc.isExited() {
			code, _ := r.proc.wait(0)
			r.line(fmt.Sprintf("exit code %d (exited before shutdown)", code))
			r.exitDone = true
		} else {
			r.shutdown("SIGTERM")
		}
	}
	if left := r.rec.Take(true); len(left) > 0 {
		r.line("## proxy requests not taken by a step")
		r.printProxy(left)
	}
	for _, name := range r.clientNames() {
		r.clients[name].close()
	}
}

// dropScrapeLog reports whether l is the request log of an extra metrics
// scrape (and counts it off).
func (r *run) dropScrapeLog(l string) bool {
	v, err := DecodeJSON([]byte(l))
	if err != nil {
		return false
	}
	m, _ := v.(map[string]any)
	if m["message"] != "http request" {
		return false
	}
	p, _ := m["path"].(string)
	if r.extraScrapes[p] > 0 {
		r.extraScrapes[p]--
		return true
	}
	return false
}

func (r *run) write() error {
	dir := filepath.Join(r.opts.Out, r.id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "transcript.txt"), []byte(joinLines(r.out)), 0o644); err != nil {
		return err
	}
	if len(r.metrics) > 0 {
		if err := os.WriteFile(filepath.Join(dir, "metrics.txt"), []byte(joinLines(r.metrics)), 0o644); err != nil {
			return err
		}
	}
	if r.proc != nil {
		r.norm.Freeze()
		all := r.proc.linesFrom(0)
		var before, after []string
		for i, l := range all {
			if r.dropScrapeLog(l) {
				continue
			}
			nl := r.norm.normalizeLogLine(l)
			if r.sigIndex >= 0 && i >= r.sigIndex {
				// Debug lines logged during shutdown go through an
				// asynchronous handler that the process may exit before
				// draining: only info and above are stable.
				if strings.HasPrefix(nl, "{") && (strings.Contains(nl, `"level":"debug"`) || strings.Contains(nl, `"level":"trace"`)) {
					continue
				}
				after = append(after, nl)
			} else {
				before = append(before, nl)
			}
		}
		var b strings.Builder
		b.WriteString("# log lines while running (sorted, counted)\n")
		b.WriteString(joinLines(sortedCounted(before)))
		b.WriteString("# log lines after the shutdown signal, info and above (sorted, counted)\n")
		b.WriteString(joinLines(sortedCounted(after)))
		if err := os.WriteFile(filepath.Join(dir, "logs.txt"), []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	for name, content := range r.files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
