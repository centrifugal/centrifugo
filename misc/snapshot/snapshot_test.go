package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizerUUIDNumbering(t *testing.T) {
	n := NewNormalizer()
	a := "0b2b6f5e-1a2b-4c3d-8e9f-0123456789ab"
	b := "11111111-2222-4333-8444-555555555555"
	require.Equal(t, "<uuid-1> <uuid-2> <uuid-1>", n.String(a+" "+b+" "+a))
	n.Freeze()
	require.Equal(t, "<uuid-2> <uuid>", n.String(b+" 99999999-2222-4333-8444-555555555555"))
}

func TestNormalizerLiteralsAndPatterns(t *testing.T) {
	n := NewNormalizer()
	n.AddLiteral("/tmp/run-123", "<tmp>")
	n.AddLiteral("127.0.0.1:40001", "127.0.0.1:<port:http>")
	require.Equal(t, "<tmp>/config.json", n.String("/tmp/run-123/config.json"))
	require.Equal(t, "listen 127.0.0.1:<port:http> from 127.0.0.1:<port>", n.String("listen 127.0.0.1:40001 from 127.0.0.1:53211"))
	require.Equal(t, "at <time> took <duration>", n.String("at 2026-10-31T10:11:12.123Z took 1.5ms"))
	// Config durations are content, not measurements.
	require.Equal(t, "timeout 25s", n.String("timeout 25s"))
	require.Equal(t, "took 1.5ms", n.StringNoDurations("took 1.5ms"))
	require.Equal(t, "token <jwt>", n.String("token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc_-1"))
	require.Equal(t, `id:1 connect:{token:"x"}`, n.String(`id:1  connect:{token:"x"}`))
}

func TestNormalizerJSON(t *testing.T) {
	n := NewNormalizer()
	got := n.NormalizeJSONBytes([]byte(`{"z":1,"epoch":"abcd","offset":3,"presence":{"0b2b6f5e-1a2b-4c3d-8e9f-0123456789ab":{"client":"0b2b6f5e-1a2b-4c3d-8e9f-0123456789ab"}},"ttl":3599,"exp":0}`))
	require.Equal(t, `{"epoch":"<epoch>","exp":0,"offset":3,"presence":{"<uuid-1>":{"client":"<uuid-1>"}},"ttl":"<ttl>","z":1}`, got)
	require.Equal(t, "raw:not json <uuid-1>", n.NormalizeJSONBytes([]byte("not json 0b2b6f5e-1a2b-4c3d-8e9f-0123456789ab")))
}

func TestNormalizeLogLine(t *testing.T) {
	n := NewNormalizer()
	line := `{"level":"info","time":"2026-10-31T10:11:12Z","pid":1234,"message":"starting","addr":"127.0.0.1:5555"}`
	require.Equal(t, `{"addr":"127.0.0.1:<port>","level":"info","message":"starting","pid":"<pid>"}`, n.normalizeLogLine(line))
	require.Equal(t, "text: panic at <time>", n.normalizeLogLine("panic at 2026-10-31 10:11:12"))
	require.Equal(t, []string{"2x a", "b"}, sortedCounted([]string{"b", "a", "a"}))
}

func TestNormalizeMetrics(t *testing.T) {
	text := `# HELP centrifugo_node_messages_sent_count Number of messages sent.
# TYPE centrifugo_node_messages_sent_count counter
centrifugo_node_messages_sent_count{type="publication",channel_namespace=""} 4
# HELP centrifugo_api_command_duration_seconds Duration.
# TYPE centrifugo_api_command_duration_seconds histogram
centrifugo_api_command_duration_seconds_bucket{method="publish",le="0.1"} 3
centrifugo_api_command_duration_seconds_sum{method="publish"} 0.0123
centrifugo_api_command_duration_seconds_count{method="publish"} 3
# TYPE go_goroutines gauge
go_goroutines 42
# TYPE centrifugo_node_build gauge
centrifugo_node_build{version="1.2.3",node="11111111-2222-4333-8444-555555555555"} 1
`
	n := NewNormalizer()
	got := normalizeMetrics(text, n, nil)
	require.Equal(t, []string{
		"# HELP centrifugo_api_command_duration_seconds Duration.",
		"# TYPE centrifugo_api_command_duration_seconds histogram",
		`centrifugo_api_command_duration_seconds_bucket{le="0.1",method="publish"} <v>`,
		`centrifugo_api_command_duration_seconds_count{method="publish"} 3`,
		`centrifugo_api_command_duration_seconds_sum{method="publish"} <v>`,
		"# TYPE centrifugo_node_build gauge",
		`centrifugo_node_build{node="<uuid-1>",version="1.2.3"} 1`,
		"# HELP centrifugo_node_messages_sent_count Number of messages sent.",
		"# TYPE centrifugo_node_messages_sent_count counter",
		`centrifugo_node_messages_sent_count{channel_namespace="",type="publication"} 4`,
		"# TYPE go_goroutines gauge",
		"go_goroutines <v>",
	}, got)
	// A renamed label shows up.
	renamed := strings.Replace(text, `{type="publication"`, `{kind="publication"`, 1)
	require.NotEqual(t, got, normalizeMetrics(renamed, NewNormalizer(), nil))
}

func TestMergePatch(t *testing.T) {
	base, err := DecodeJSON([]byte(`{"a":{"b":1,"c":2},"list":[1,2],"keep":true}`))
	require.NoError(t, err)
	out, err := mergeRaw(base, []byte(`{"a":{"b":null,"d":3},"list":[3]}`))
	require.NoError(t, err)
	require.Equal(t, `{"a":{"c":2,"d":3},"keep":true,"list":[3]}`, CanonicalJSON(out))
}

func TestExpandJSON(t *testing.T) {
	out, err := expandJSON([]byte(`{"token":{"$jwt":{"sub":"alice"},"secret":"s"},"b":{"$b64":"hi"},"t":"$now+10","plain":"$x"}`))
	require.NoError(t, err)
	v, err := DecodeJSON(out)
	require.NoError(t, err)
	m := v.(map[string]any)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("hi")), m["b"])
	require.Equal(t, "$x", m["plain"])
	tok := m["token"].(string)
	require.Len(t, strings.Split(tok, "."), 3)
	want, err := signHS256(map[string]any{"sub": "alice"}, "s")
	require.NoError(t, err)
	require.Equal(t, want, tok)
}

func TestSnakeCase(t *testing.T) {
	require.Equal(t, "sub_refresh", snakeCase("SubRefresh"))
	require.Equal(t, "rpc", snakeCase("RPC"))
	require.Equal(t, "connect", snakeCase("Connect"))
}

func TestUnifiedDiff(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	b := "1\n2\n3\n4\nfive\n6\n7\n8\n9\n10\neleven\n"
	require.Equal(t, `--- a/x
+++ b/x
@@ -2,9 +2,10 @@
 2
 3
 4
-5
+five
 6
 7
 8
 9
 10
+eleven
`, UnifiedDiff("a/x", "b/x", a, b, 3))
}

func TestDiffDirs(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write := func(dir, name, content string) {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	write(a, "s/transcript.txt", "x\ny\n")
	write(b, "s/transcript.txt", "x\ny\n")
	d, err := DiffDirs(a, b)
	require.NoError(t, err)
	require.Empty(t, d)

	write(b, "s/transcript.txt", "x\nz\n")
	write(a, "only_a.txt", "1\n")
	d, err = DiffDirs(a, b)
	require.NoError(t, err)
	require.Contains(t, d, "Only in "+a+": only_a.txt")
	require.Contains(t, d, "-y\n+z\n")
}

func TestScenariosParse(t *testing.T) {
	scenarios, _, err := loadScenarios([]string{"scenarios"})
	require.NoError(t, err)
	require.NotEmpty(t, scenarios)
	known := map[string]bool{
		"connect": true, "send": true, "pushes": true, "wait_close": true, "close": true, "uni": true,
		"api": true, "grpc_api": true, "http": true, "routes": true, "metrics": true, "wait_proxy": true,
		"proxy_rules": true, "wait_log": true, "reload": true, "sql": true, "exec": true, "shutdown": true,
	}
	for _, s := range scenarios {
		for _, st := range append(append(append([]Step{}, s.Setup...), s.Steps...), s.Teardown...) {
			require.True(t, known[st.Op], "%s: unknown op %q", s.Name, st.Op)
		}
	}
}
