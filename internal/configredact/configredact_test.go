package configredact

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
)

func TestMaskSecret(t *testing.T) {
	cases := map[string]string{
		"":                                       "••••", // empty handled by caller; masker still safe
		"short":                                  "••••",
		"dev-secret":                             "••••", // 10 runes < 12 → fully masked
		"8a61bf3d-1a1a-410f-8185-b6582c530e852s": "8a…2s",
	}
	for in, want := range cases {
		if got := maskSecret(in); got != want {
			t.Errorf("maskSecret(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShowPrefix(t *testing.T) {
	if got := showPrefix("default", true); got != "def…" {
		t.Errorf("showPrefix(default) = %q, want def…", got)
	}
	if got := showPrefix("abc", true); got != "•••" { // too short → masked fully
		t.Errorf("showPrefix(abc) = %q, want •••", got)
	}
}

func TestMaskURL(t *testing.T) {
	// query params and userinfo password masked; scheme/host/keys kept.
	kind, disp := maskURL("tcp://user:supersecret@127.0.0.1:9000?username=default&password=longpassword&sslmode=require", true)
	if kind != "url" {
		t.Fatalf("kind = %q, want url", kind)
	}
	want := "tcp://user:sup…@127.0.0.1:9000?username=def…&password=lon…&sslmode=req…"
	if disp != want {
		t.Errorf("maskURL display:\n got %q\nwant %q", disp, want)
	}

	// no credentials → shown as-is.
	if _, disp := maskURL("http://localhost:3000", true); disp != "http://localhost:3000" {
		t.Errorf("clean url changed: %q", disp)
	}

	// unparsable / non-URL DSN → fail closed to a masked secret.
	if kind, _ := maskURL("host=db port=5432 password=secretpass", true); kind != "secret" {
		t.Errorf("non-url kind = %q, want secret (fail closed)", kind)
	}
}

type dur time.Duration

func (d dur) String() string { return time.Duration(d).String() }

type sub struct {
	Key string `json:"key"` // secret by default
}

type sample struct {
	Name     string            `json:"name" expose:"full" doc:"Service name."`
	Password string            `json:"password"`          // secret by default
	DSN      string            `json:"dsn" expose:"url"`  //
	Empty    string            `json:"empty"`             // empty secret
	Port     int               `json:"port"`              // number, shown
	Enabled  bool              `json:"enabled"`           // bool, shown
	TTL      dur               `json:"ttl"`               // Stringer → text
	DSNs     []string          `json:"dsns" expose:"url"` // array of urls
	Headers  map[string]string `json:"headers"`           // map → values masked, keys shown
	Sub      sub               `json:"sub"`
	Ignored  string            `json:"-"`                 // skipped (json)
	Hidden   string            `json:"hidden" expose:"-"` // skipped (NODOC)
}

func find(nodes []Node, key string) *Node {
	for i := range nodes {
		if nodes[i].Key == key {
			return &nodes[i]
		}
	}
	return nil
}

func TestBuild(t *testing.T) {
	eff := sample{
		Name:     "svc",
		Password: "supersecretvalue",
		DSN:      "tcp://127.0.0.1:9000?password=longpassword",
		Empty:    "",
		Port:     9000,
		Enabled:  true,
		TTL:      dur(720 * time.Hour),
		DSNs:     []string{"redis://u:longpassword@host:6379/0"},
		Headers:  map[string]string{"Authorization": "Bearer longtokenvalue", "Content-Type": "application/json"},
		Sub:      sub{Key: "anothersecretkey"},
	}
	// defaults: Port matches (not overridden), everything else differs.
	def := sample{Port: 9000}

	resp := Build(eff, def, Options{Peek: true})
	f := resp.Fields

	if n := find(f, "name"); n == nil || n.Kind != "string" || n.Value != "svc" {
		t.Errorf("name node wrong: %+v", n)
	}
	if n := find(f, "ignored"); n != nil {
		t.Errorf("json:\"-\" field should be skipped")
	}
	if n := find(f, "hidden"); n != nil {
		t.Errorf("expose:\"-\" (NODOC) field should be skipped")
	}

	if n := find(f, "password"); n == nil || n.Kind != "secret" || n.Set == nil || !*n.Set || n.Display == nil || *n.Display != "su…ue" {
		t.Errorf("password node wrong: %+v (display=%v)", n, n.Display)
	}
	if n := find(f, "empty"); n == nil || n.Kind != "secret" || n.Set == nil || *n.Set {
		t.Errorf("empty secret should have set=false: %+v", n)
	}
	if n := find(f, "dsn"); n == nil || n.Kind != "url" || n.Display == nil || *n.Display != "tcp://127.0.0.1:9000?password=lon…" {
		t.Errorf("dsn node wrong: %+v (display=%v)", n, derefStr(n))
	}
	if n := find(f, "port"); n == nil || n.Kind != "number" || n.Overridden {
		t.Errorf("port should be number and not overridden: %+v", n)
	}
	if n := find(f, "name"); n == nil || !n.Overridden {
		t.Errorf("name should be overridden")
	}
	if n := find(f, "enabled"); n == nil || n.Kind != "bool" || n.Value != true {
		t.Errorf("enabled node wrong: %+v", n)
	}
	if n := find(f, "ttl"); n == nil || n.Kind != "string" || n.Value != "720h0m0s" {
		t.Errorf("ttl (Stringer) node wrong: %+v", n)
	}

	dsns := find(f, "dsns")
	if dsns == nil || dsns.Kind != "array" || len(dsns.Items) != 1 || dsns.Items[0].Kind != "url" {
		t.Fatalf("dsns array wrong: %+v", dsns)
	}
	if dsns.Items[0].Display == nil || *dsns.Items[0].Display != "redis://u:lon…@host:6379/0" {
		t.Errorf("dsns[0] masking wrong: %v", derefStr(&dsns.Items[0]))
	}

	headers := find(f, "headers")
	if headers == nil || headers.Kind != "object" || len(headers.Children) != 2 {
		t.Fatalf("headers map wrong: %+v", headers)
	}
	// keys shown, all values masked (sorted: Authorization, Content-Type)
	if headers.Children[0].Key != "Authorization" || headers.Children[0].Kind != "secret" || headers.Children[0].Display == nil {
		t.Errorf("Authorization header should be masked secret: %+v", headers.Children[0])
	}
	if headers.Children[1].Key != "Content-Type" || headers.Children[1].Kind != "secret" {
		t.Errorf("Content-Type value should also be masked: %+v", headers.Children[1])
	}

	subN := find(f, "sub")
	if subN == nil || subN.Kind != "object" {
		t.Fatalf("sub object wrong: %+v", subN)
	}
	if k := find(subN.Children, "key"); k == nil || k.Kind != "secret" {
		t.Errorf("sub.key should be secret: %+v", k)
	}
}

func TestDocTag(t *testing.T) {
	resp := Build(sample{Name: "x"}, sample{}, Options{})
	n := find(resp.Fields, "name")
	if n == nil || n.Doc == nil {
		t.Fatalf("name node should carry doc: %+v", n)
	}
	if n.Doc.Description != "Service name." || n.Doc.Type != "string" || n.Doc.Env != "CENTRIFUGO_NAME" {
		t.Errorf("doc wrong: %+v", n.Doc)
	}
}

func TestEnvOnlyForNonComplexTypes(t *testing.T) {
	type leaf struct {
		Key string `json:"key" expose:"full"`
	}
	type spec struct {
		Name    string            `json:"name" expose:"full"` // scalar leaf → env
		Port    int               `json:"port"`               // scalar leaf → env
		Tags    []string          `json:"tags" expose:"full"` // []scalar → env
		Headers map[string]string `json:"headers"`            // map → env
		Obj     leaf              `json:"obj"`                // struct → no env
		List    []leaf            `json:"list"`               // []struct → no env
	}
	resp := Build(spec{Obj: leaf{Key: "x"}, List: []leaf{{Key: "y"}}}, spec{}, Options{})

	withEnv := func(key string) {
		if n := find(resp.Fields, key); n == nil || n.Doc == nil || n.Doc.Env == "" {
			t.Errorf("%s should carry an env var: %+v", key, n)
		}
	}
	noEnv := func(key string) {
		if n := find(resp.Fields, key); n == nil || n.Doc == nil || n.Doc.Env != "" {
			t.Errorf("%s (object-like) should NOT carry an env var: %+v", key, n)
		}
	}
	withEnv("name")
	withEnv("port")
	withEnv("tags")
	withEnv("headers")
	noEnv("obj")
	noEnv("list")

	// Leaf keys inside an object still get their own env var.
	if obj := find(resp.Fields, "obj"); obj != nil {
		if k := find(obj.Children, "key"); k == nil || k.Doc == nil || k.Doc.Env != "CENTRIFUGO_OBJ_KEY" {
			t.Errorf("obj.key should have env CENTRIFUGO_OBJ_KEY: %+v", k)
		}
	}
}

func TestEnvForNamedCollectionElements(t *testing.T) {
	// A named-collection element: its `name` becomes part of the env var for each
	// leaf field, mirroring CENTRIFUGO_<ARRAY>_<NAME>_<FIELD>.
	type namespace struct {
		Name           string `json:"name" expose:"full"`
		PresenceJoin   bool   `json:"presence_join_leave"`
		SubscribeProxy string `json:"subscribe_proxy_name" expose:"full"`
	}
	// A non-named element (e.g. a rate-limit override): no per-element env var.
	type override struct {
		NamespaceName string `json:"namespace_name" expose:"full"`
		Rate          int    `json:"rate"`
	}
	type channel struct {
		Namespaces []namespace `json:"namespaces"`
		Overrides  []override  `json:"overrides"`
	}
	type cfg struct {
		Channel channel `json:"channel"`
	}

	resp := Build(cfg{Channel: channel{
		Namespaces: []namespace{{Name: "chat-1", SubscribeProxy: "default"}},
		Overrides:  []override{{NamespaceName: "chat-1", Rate: 5}},
	}}, cfg{}, Options{})

	ch := find(resp.Fields, "channel")
	ns := find(ch.Children, "namespaces")
	if ns == nil || len(ns.Items) != 1 {
		t.Fatalf("namespaces wrong: %+v", ns)
	}
	nsEl := ns.Items[0]
	// "chat-1" → name segment "CHAT_1" (dash → underscore, like NameForEnv).
	if n := find(nsEl.Children, "subscribe_proxy_name"); n == nil || n.Doc == nil ||
		n.Doc.Env != "CENTRIFUGO_CHANNEL_NAMESPACES_CHAT_1_SUBSCRIBE_PROXY_NAME" {
		t.Errorf("namespace field env wrong: %+v", derefDoc(n))
	}
	if n := find(nsEl.Children, "presence_join_leave"); n == nil || n.Doc == nil ||
		n.Doc.Env != "CENTRIFUGO_CHANNEL_NAMESPACES_CHAT_1_PRESENCE_JOIN_LEAVE" {
		t.Errorf("namespace bool env wrong: %+v", derefDoc(n))
	}

	// Non-named collection element: fields carry no env var.
	ovs := find(ch.Children, "overrides")
	if ovs == nil || len(ovs.Items) != 1 {
		t.Fatalf("overrides wrong: %+v", ovs)
	}
	if n := find(ovs.Items[0].Children, "rate"); n == nil || n.Doc == nil || n.Doc.Env != "" {
		t.Errorf("override field should have no env var: %+v", derefDoc(n))
	}
}

func TestEnvSuppressedInsideMap(t *testing.T) {
	type entry struct {
		Weight int `json:"weight"`
	}
	type cfg struct {
		Entries map[string]entry `json:"entries"`
	}
	resp := Build(cfg{Entries: map[string]entry{"a": {Weight: 1}}}, cfg{}, Options{})
	entries := find(resp.Fields, "entries")
	if entries == nil || len(entries.Children) != 1 {
		t.Fatalf("entries wrong: %+v", entries)
	}
	if n := find(entries.Children[0].Children, "weight"); n == nil || n.Doc == nil || n.Doc.Env != "" {
		t.Errorf("field inside a map value should have no env var: %+v", derefDoc(n))
	}
}

func derefDoc(n *Node) any {
	if n == nil || n.Doc == nil {
		return n
	}
	return n.Doc.Env
}

func TestOverriddenInSliceElement(t *testing.T) {
	type opt struct {
		Name     string `json:"name" expose:"full"`
		Presence bool   `json:"presence"`
	}
	type cfg struct {
		Items []opt `json:"items"`
	}
	// effective has one configured element; defaults has an empty slice.
	resp := Build(cfg{Items: []opt{{Name: "chat"}}}, cfg{}, Options{})
	items := find(resp.Fields, "items")
	if items == nil || len(items.Items) != 1 {
		t.Fatalf("items wrong: %+v", items)
	}
	el := items.Items[0]
	if n := find(el.Children, "name"); n == nil || !n.Overridden {
		t.Errorf("name=chat should be overridden")
	}
	if n := find(el.Children, "presence"); n == nil || n.Overridden {
		t.Errorf("presence=false (zero value) should NOT be overridden: %+v", n)
	}
}

func TestOverriddenUsesDefaultTagInSliceElement(t *testing.T) {
	type opt struct {
		// Mirrors real fields: a string with a non-zero default ("default") and a
		// duration with a default ("1s"), both inside a slice element that the
		// defaults-only config does not materialize.
		Name         string `json:"name" expose:"full"`
		ProxyName    string `json:"proxy_name" expose:"full" default:"default"`
		DialTimeout  dur    `json:"dial_timeout" default:"1s"`
		WriteTimeout dur    `json:"write_timeout" default:"1s"`
	}
	type cfg struct {
		Items []opt `json:"items"`
	}
	// Effective: defaults applied per-element (as GetConfig does), plus one field
	// the user actually changed. Defaults config has an empty slice.
	resp := Build(cfg{Items: []opt{{
		Name:         "chat",
		ProxyName:    "default",
		DialTimeout:  dur(time.Second),
		WriteTimeout: dur(2 * time.Second), // changed from default
	}}}, cfg{}, Options{})

	items := find(resp.Fields, "items")
	if items == nil || len(items.Items) != 1 {
		t.Fatalf("items wrong: %+v", items)
	}
	el := items.Items[0]

	// proxy_name == its default tag → NOT overridden (the reported bug).
	if n := find(el.Children, "proxy_name"); n == nil || n.Overridden {
		t.Errorf("proxy_name=default equals its default tag, should NOT be overridden: %+v", n)
	}
	// dial_timeout == "1s" default tag → NOT overridden (the reported bug).
	if n := find(el.Children, "dial_timeout"); n == nil || n.Overridden {
		t.Errorf("dial_timeout=1s equals its default tag, should NOT be overridden: %+v", n)
	}
	// write_timeout differs from its default → still flagged.
	if n := find(el.Children, "write_timeout"); n == nil || !n.Overridden {
		t.Errorf("write_timeout=2s differs from default 1s, should be overridden: %+v", n)
	}
	// name has no default tag and is non-zero → overridden via zero-value fallback.
	if n := find(el.Children, "name"); n == nil || !n.Overridden {
		t.Errorf("name=chat should be overridden: %+v", n)
	}
}

func TestOverriddenSliceDefaultInSliceElement(t *testing.T) {
	type redis struct {
		// Mirrors configtypes Redis: a []string whose whole-slice default is
		// encoded in the tag (space-separated elements).
		Address []string `json:"address" expose:"url" default:"redis://127.0.0.1:6379"`
	}
	type consumer struct {
		Name  string `json:"name" expose:"full"`
		Redis redis  `json:"redis"`
	}
	type cfg struct {
		Consumers []consumer `json:"consumers"`
	}
	// Effective consumer has the default address (applied per-element); defaults
	// config has no consumers, so there is no slice counterpart to diff against.
	resp := Build(cfg{Consumers: []consumer{{
		Name:  "stream",
		Redis: redis{Address: []string{"redis://127.0.0.1:6379"}},
	}}}, cfg{}, Options{})

	consumers := find(resp.Fields, "consumers")
	if consumers == nil || len(consumers.Items) != 1 {
		t.Fatalf("consumers wrong: %+v", consumers)
	}
	redisN := find(consumers.Items[0].Children, "redis")
	if redisN == nil {
		t.Fatalf("redis object missing")
	}
	addr := find(redisN.Children, "address")
	if addr == nil || addr.Kind != "array" || len(addr.Items) != 1 {
		t.Fatalf("address array wrong: %+v", addr)
	}
	// The single default address must NOT be flagged (the reported bug).
	if addr.Items[0].Overridden {
		t.Errorf("address[0]=default redis url should NOT be overridden: %+v", addr.Items[0])
	}

	// A non-default address (and an extra element beyond the default) is flagged.
	resp2 := Build(cfg{Consumers: []consumer{{
		Name:  "stream",
		Redis: redis{Address: []string{"redis://other:6379", "redis://127.0.0.1:6379"}},
	}}}, cfg{}, Options{})
	addr2 := find(find(find(resp2.Fields, "consumers").Items[0].Children, "redis").Children, "address")
	if addr2 == nil || len(addr2.Items) != 2 {
		t.Fatalf("address2 wrong: %+v", addr2)
	}
	if !addr2.Items[0].Overridden {
		t.Errorf("address[0]=other should be overridden: %+v", addr2.Items[0])
	}
	if !addr2.Items[1].Overridden {
		t.Errorf("address[1] beyond the single default should be overridden: %+v", addr2.Items[1])
	}
}

func derefStr(n *Node) string {
	if n.Display == nil {
		return "<nil>"
	}
	return *n.Display
}

func TestBuildWithoutPeek(t *testing.T) {
	type cfg struct {
		Password string            `json:"password"`
		DSN      string            `json:"dsn" expose:"url"`
		BadDSN   string            `json:"bad_dsn" expose:"url"`
		Clean    string            `json:"clean" expose:"url"`
		Name     string            `json:"name" expose:"full"`
		Headers  map[string]string `json:"headers"`
		Port     int               `json:"port"`
	}
	resp := Build(cfg{
		Password: "supersecretvalue",
		DSN:      "redis://u:longpassword@host:6379/0?token=longtokenvalue",
		BadDSN:   "host=db port=5432 password=secretpass",
		Clean:    "http://localhost:3000",
		Name:     "svc",
		Headers:  map[string]string{"Authorization": "Bearer longtokenvalue"},
		Port:     9000,
	}, cfg{}, Options{})
	f := resp.Fields

	if n := find(f, "password"); n == nil || n.Kind != "secret" || n.Set == nil || !*n.Set || n.Display != nil {
		t.Errorf("password without peek should show presence only: %+v", n)
	}
	if n := find(f, "dsn"); n == nil || n.Kind != "url" || n.Display == nil || *n.Display != "redis://u:•••@host:6379/0?token=•••" {
		t.Errorf("dsn without peek wrong: %+v (display=%v)", n, derefStr(n))
	}
	if n := find(f, "bad_dsn"); n == nil || n.Kind != "secret" || n.Set == nil || !*n.Set || n.Display != nil {
		t.Errorf("unparsable url without peek should fail closed to presence only: %+v", n)
	}
	if n := find(f, "clean"); n == nil || n.Kind != "url" || n.Display == nil || *n.Display != "http://localhost:3000" {
		t.Errorf("url without credentials should be shown: %+v", n)
	}
	if n := find(f, "name"); n == nil || n.Value != "svc" {
		t.Errorf("full field should be shown: %+v", n)
	}
	headers := find(f, "headers")
	if headers == nil || len(headers.Children) != 1 || headers.Children[0].Kind != "secret" || headers.Children[0].Display != nil {
		t.Errorf("header value without peek should show presence only: %+v", headers)
	}
	if n := find(f, "port"); n == nil || n.Value != 9000 {
		t.Errorf("numbers are shown: %+v", n)
	}
	if n := find(f, "password"); n == nil || n.Doc == nil {
		t.Errorf("docs are included by default: %+v", n)
	}
}

func TestBuildOmitDocs(t *testing.T) {
	type inner struct {
		Key string `json:"key" doc:"Inner key."`
	}
	type cfg struct {
		Name  string  `json:"name" expose:"full" doc:"Name."`
		Inner inner   `json:"inner"`
		List  []inner `json:"list"`
	}
	resp := Build(cfg{Name: "x", List: []inner{{Key: "k"}}}, cfg{}, Options{OmitDocs: true})
	var walk func(nodes []Node)
	walk = func(nodes []Node) {
		for _, n := range nodes {
			if n.Doc != nil {
				t.Errorf("node %q carries doc with OmitDocs", n.Key)
			}
			walk(n.Children)
			walk(n.Items)
		}
	}
	walk(resp.Fields)
	if n := find(resp.Fields, "name"); n == nil || n.Value != "x" {
		t.Errorf("values are unchanged by OmitDocs: %+v", n)
	}
}

func TestValue(t *testing.T) {
	peek := Options{Peek: true}
	cases := []struct {
		s, expose string
		opts      Options
		want      string
	}{
		{"", "", Options{}, ""},
		{"", "-", peek, ""},
		{"application/json", "full", Options{}, "application/json"},
		{"Bearer longtokenvalue", "", Options{}, "••••"},
		{"Bearer longtokenvalue", "", peek, "Be…ue"},
		{"short", "", peek, "••••"},
		{"Bearer longtokenvalue", "unknown", Options{}, "••••"},
		{"Bearer longtokenvalue", "-", Options{}, "••••"},
		{"Bearer longtokenvalue", "-", peek, "••••"},
		{"redis://u:longpassword@host:6379", "url", Options{}, "redis://u:•••@host:6379"},
		{"redis://u:longpassword@host:6379", "url", peek, "redis://u:lon…@host:6379"},
		{"http://localhost:3000", "url", Options{}, "http://localhost:3000"},
		{"nats://sometoken@host:4222", "url", Options{}, "nats://•••@host:4222"},
		{"nats://sometoken@host:4222", "url", peek, "nats://som…@host:4222"},
		{"https://apikeyvalue@host/x", "url", Options{}, "https://•••@host/x"},
		{"https://host/p?tokenvalue", "url", Options{}, "https://host/p?•••"},
		{"https://host/p?a=1&tokenvalue", "url", peek, "https://host/p?a=•••&tok…"},
		{"redis://:pw1@h1:6379,redis://:pw2secret@h2:6379", "url", Options{}, "••••"},
		{"host=db password=secretpass", "url", Options{}, "••••"},
		{"host=db password=secretpass", "url", peek, "ho…ss"},
	}
	for _, c := range cases {
		if got := Value(c.s, c.expose, c.opts); got != c.want {
			t.Errorf("Value(%q, %q, %+v) = %q, want %q", c.s, c.expose, c.opts, got, c.want)
		}
	}
}

// TestBuildRealConfig runs the walker over the full Centrifugo config with
// secrets set and checks that none of them appears in the output.
func TestBuildRealConfig(t *testing.T) {
	const secret = "s3cr3t-value-that-must-not-leak"
	eff := config.DefaultConfig()
	eff.Client.Token.HMACSecretKey = secret
	eff.HttpAPI.Key = secret
	eff.Admin.Password = secret
	eff.Admin.Secret = secret
	eff.Engine.Redis.Address = []string{"redis://user:" + secret + "@127.0.0.1:6379"}
	eff.PidFile = "/var/run/centrifugo.pid"

	for _, opts := range []Options{{}, {Peek: true}, {OmitDocs: true}} {
		data, err := json.Marshal(Build(eff, config.DefaultConfig(), opts))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), secret) {
			t.Fatalf("secret leaked with %+v", opts)
		}
		// Peek shows at most a few runes of the secret.
		if strings.Contains(string(data), secret[:6]) {
			t.Fatalf("too much of the secret shown with %+v", opts)
		}
		if !strings.Contains(string(data), `"/var/run/centrifugo.pid"`) {
			t.Fatalf("exposed field missing with %+v", opts)
		}
		if !strings.Contains(string(data), `redis://user:`) {
			t.Fatalf("url field with masked credentials missing with %+v", opts)
		}
	}
}
