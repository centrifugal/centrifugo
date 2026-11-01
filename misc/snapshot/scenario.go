package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// Scenario is one file in a scenarios directory. See README.md.
type Scenario struct {
	// Name is the file name without extension; output goes to <out>/<Name>[@variant].
	Name        string `json:"-"`
	Description string `json:"description"`
	// Server false runs only "exec" steps without starting the binary as a server.
	Server *bool `json:"server"`
	// Requires lists integration services (redis, postgres, kafka, nats) the
	// scenario needs; it is skipped unless all are enabled with -with.
	Requires []string `json:"requires"`
	// Config is a JSON merge patch applied over the base config(s).
	Config json.RawMessage `json:"config"`
	// Env is added to the server process environment.
	Env map[string]string `json:"env"`
	// Proxy holds the initial scripted proxy rules.
	Proxy []ProxyRule `json:"proxy"`
	// Variants run the same steps with different config/requirements.
	Variants map[string]Variant `json:"variants"`
	// Setup steps run before the server starts (e.g. sql), teardown after it stopped.
	Setup    []Step `json:"setup"`
	Steps    []Step `json:"steps"`
	Teardown []Step `json:"teardown"`
}

type Variant struct {
	Requires []string          `json:"requires"`
	Config   json.RawMessage   `json:"config"`
	Env      map[string]string `json:"env"`
}

// ProxyRule scripts one proxy answer. The first rule whose Kind and Match
// fit the request is used.
type ProxyRule struct {
	// Kind is the HTTP path without leading slash (as set in the endpoint
	// in config) or the snake_case gRPC method name (connect, refresh,
	// subscribe, publish, rpc, sub_refresh, ...).
	Kind string `json:"kind"`
	// Match maps dotted paths in the request JSON to expected values.
	Match map[string]any `json:"match"`
	// Status is the HTTP status code (default 200).
	Status int `json:"status"`
	// Body is the response JSON (HTTP), and for gRPC the protojson of the
	// response message unless GRPCBody is set.
	Body     json.RawMessage `json:"body"`
	GRPCBody json.RawMessage `json:"grpc_body"`
	// GRPCCode, when non-zero, makes the gRPC proxy return this status code.
	GRPCCode    int    `json:"grpc_code"`
	GRPCMessage string `json:"grpc_message"`
}

// Step is one action. Op selects which fields are used.
type Step struct {
	Op     string `json:"op"`
	Client string `json:"client"`
	// connect
	Transport string            `json:"transport"`
	Headers   map[string]string `json:"headers"`
	Query     string            `json:"query"`
	// send
	Cmd     json.RawMessage `json:"cmd"`
	NoReply bool            `json:"no_reply"`
	// pushes, wait_proxy
	Count int    `json:"count"`
	Sort  bool   `json:"sort"`
	Kind  string `json:"kind"`
	// api, grpc_api, http
	Method   string            `json:"method"`
	Body     json.RawMessage   `json:"body"`
	RawBody  *string           `json:"raw_body"`
	Auth     string            `json:"auth"`
	Port     string            `json:"port"`
	Path     string            `json:"path"`
	Metadata map[string]string `json:"metadata"`
	// routes
	Paths   []string `json:"paths"`
	Methods []string `json:"methods"`
	// metrics
	Filter string `json:"filter"`
	// wait_log, reload
	Match []string `json:"match"`
	// reload: merge patch over the current config, or a raw file content.
	Config    json.RawMessage `json:"config"`
	RawConfig *string         `json:"raw_config"`
	// proxy_rules
	Rules []ProxyRule `json:"rules"`
	// sql
	DSN  string `json:"dsn"`
	Exec string `json:"exec"`
	// exec
	Args      []string          `json:"args"`
	Files     map[string]string `json:"files"`
	ReadFiles []string          `json:"read_files"`
	Stdin     string            `json:"stdin"`
	// Replace maps regexps to replacements applied to exec output (for
	// values a command generates randomly).
	Replace map[string]string `json:"replace"`
	// shutdown
	Signal string `json:"signal"`
	// Comment is copied into the transcript.
	Comment string `json:"comment"`
}

// TemplateData is available to {{...}} templates in config and scenario files.
type TemplateData struct {
	Port        map[string]int
	ProxyHTTP   string
	ProxyGRPC   string
	TmpDir      string
	Unique      string
	RedisAddr   string
	PostgresDSN string
	KafkaAddr   string
	NatsURL     string
}

func (s *Scenario) runsServer() bool {
	return s.Server == nil || *s.Server
}

// loadScenarios reads every *.json file of the given directories; later
// directories may override files of earlier ones by name.
func loadScenarios(dirs []string) ([]*Scenario, map[string][]byte, error) {
	raw := map[string][]byte{}
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			return nil, nil, err
		}
		for _, m := range matches {
			data, err := os.ReadFile(m)
			if err != nil {
				return nil, nil, err
			}
			raw[strings.TrimSuffix(filepath.Base(m), ".json")] = data
		}
	}
	names := make([]string, 0, len(raw))
	for n := range raw {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []*Scenario
	for _, n := range names {
		// Validate structure with zero template data; real parse happens per run.
		s, err := parseScenario(n, raw[n], placeholderData())
		if err != nil {
			return nil, nil, err
		}
		out = append(out, s)
	}
	return out, raw, nil
}

func parseScenario(name string, data []byte, td TemplateData) (*Scenario, error) {
	expanded, err := renderTemplate(name, string(data), td)
	if err != nil {
		return nil, err
	}
	var s Scenario
	dec := json.NewDecoder(strings.NewReader(expanded))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("scenario %s: %w", name, err)
	}
	s.Name = name
	return &s, nil
}

func renderTemplate(name, text string, td TemplateData) (string, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return buf.String(), nil
}

// mergePatch applies an RFC 7386 JSON merge patch.
func mergePatch(target, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	t, ok := target.(map[string]any)
	if !ok {
		t = map[string]any{}
	}
	for k, v := range p {
		if v == nil {
			delete(t, k)
			continue
		}
		t[k] = mergePatch(t[k], v)
	}
	return t
}

func mergeRaw(target any, patch json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(patch)) == 0 {
		return target, nil
	}
	p, err := DecodeJSON(patch)
	if err != nil {
		return nil, err
	}
	return mergePatch(target, p), nil
}

// portNames are the ports the tool allocates for each run.
var portNames = []string{"http", "internal", "grpc_api", "uni_grpc", "proxy_http", "proxy_grpc"}

// placeholderData lets scenario files be validated before a run.
func placeholderData() TemplateData {
	ports := map[string]int{}
	for i, p := range portNames {
		ports[p] = 10000 + i
	}
	return TemplateData{Port: ports}
}
