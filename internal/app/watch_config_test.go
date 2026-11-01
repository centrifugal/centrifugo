package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	watchWait = 5 * time.Second
	watchTick = 5 * time.Millisecond
)

// watchTest runs a configWatcher on a file and records its outcomes.
type watchTest struct {
	restart    chan struct{}
	rejected   chan error
	unchanged  chan struct{}
	unreadable chan error
}

func startWatcher(t *testing.T, configFile string) *watchTest {
	t.Helper()
	wt := &watchTest{
		restart:    make(chan struct{}, 1),
		rejected:   make(chan error, 16),
		unchanged:  make(chan struct{}, 16),
		unreadable: make(chan error, 16),
	}
	w := newConfigWatcher(nil, configFile)
	w.interval = 10 * time.Millisecond
	w.onRejected = func(err error) { wt.rejected <- err }
	w.onUnchanged = func() { wt.unchanged <- struct{}{} }
	w.onUnreadable = func(err error) { wt.unreadable <- err }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.run(ctx, wt.restart)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return wt
}

func (wt *watchTest) requireRestart(t *testing.T) {
	t.Helper()
	select {
	case <-wt.restart:
	case <-time.After(watchWait):
		require.Fail(t, "no restart")
	}
}

// requireNoRestart checks that the watcher signalled no restart for the
// changes evaluated so far. It makes the watcher evaluate path once more,
// with a newline appended, and waits for the outcome: a watcher which
// signalled a restart has returned and evaluates nothing more.
func (wt *watchTest) requireNoRestart(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	writeFile(t, path, string(data)+"\n")
	select {
	case <-wt.rejected:
	case <-wt.unchanged:
	case <-wt.restart:
		require.Fail(t, "restart signalled")
	case <-time.After(watchWait):
		require.Fail(t, "change not evaluated")
	}
	require.Empty(t, wt.restart)
}

func (wt *watchTest) requireRejected(t *testing.T) error {
	t.Helper()
	select {
	case err := <-wt.rejected:
		return err
	case <-time.After(watchWait):
		require.Fail(t, "change not rejected")
		return nil
	}
}

func (wt *watchTest) requireUnchanged(t *testing.T) {
	t.Helper()
	select {
	case <-wt.unchanged:
	case <-time.After(watchWait):
		require.Fail(t, "change not seen")
	}
}

func (wt *watchTest) requireUnreadable(t *testing.T) {
	t.Helper()
	select {
	case <-wt.unreadable:
	case <-time.After(watchWait):
		require.Fail(t, "file not seen unreadable")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestWatchConfigValueChange(t *testing.T) {
	path := writeConfig(t, `{"client": {"insecure": false}}`)
	wt := startWatcher(t, path)
	writeFile(t, path, `{"client": {"insecure": true}}`)
	wt.requireRestart(t)
}

func TestWatchConfigInvalidJSON(t *testing.T) {
	path := writeConfig(t, `{"client": {"insecure": false}}`)
	wt := startWatcher(t, path)
	writeFile(t, path, `{"client": {"insecure": tr`)
	require.ErrorContains(t, wt.requireRejected(t), "error reading config file")
	wt.requireNoRestart(t, path)
	writeFile(t, path, `{"client": {"insecure": true}}`)
	wt.requireRestart(t)
}

func TestWatchConfigValidationError(t *testing.T) {
	path := writeConfig(t, `{"channel": {"namespaces": [{"name": "chat"}]}}`)
	wt := startWatcher(t, path)
	writeFile(t, path, `{"channel": {"namespaces": [{"name": "bad name"}]}}`)
	require.ErrorContains(t, wt.requireRejected(t), "bad name")
	wt.requireNoRestart(t, path)
}

func TestWatchConfigNoEffectiveChange(t *testing.T) {
	path := writeConfig(t, `{"client": {"insecure": true}, "log": {"level": "debug"}}`)
	wt := startWatcher(t, path)
	writeFile(t, path, "{\n  \"log\": {\"level\": \"debug\"},\n  \"client\": {\"insecure\": true}\n}\n")
	wt.requireUnchanged(t)
	wt.requireNoRestart(t, path)
}

func TestWatchConfigAtomicSave(t *testing.T) {
	path := writeConfig(t, `{"client": {"insecure": false}}`)
	wt := startWatcher(t, path)
	tmp := filepath.Join(filepath.Dir(path), ".config.json.tmp")
	writeFile(t, tmp, `{"client": {"insecure": true}}`)
	require.NoError(t, os.Rename(tmp, path))
	wt.requireRestart(t)
}

// Kubernetes ConfigMap volume: config.json links to ..data/config.json, and
// ..data, a link to a directory, is replaced by renaming another link over it.
func TestWatchConfigSymlinkSwap(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "v1"), 0700))
	writeFile(t, filepath.Join(dir, "v1", "config.json"), `{"client": {"insecure": false}}`)
	require.NoError(t, os.Symlink("v1", filepath.Join(dir, "..data")))
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.Symlink(filepath.Join("..data", "config.json"), path))

	wt := startWatcher(t, path)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "v2"), 0700))
	writeFile(t, filepath.Join(dir, "v2", "config.json"), `{"client": {"insecure": true}}`)
	require.NoError(t, os.Symlink("v2", filepath.Join(dir, "..data_tmp")))
	require.NoError(t, os.Rename(filepath.Join(dir, "..data_tmp"), filepath.Join(dir, "..data")))
	wt.requireRestart(t)
}

func TestWatchConfigFileRemoved(t *testing.T) {
	const content = `{"client": {"insecure": false}}`
	path := writeConfig(t, content)
	wt := startWatcher(t, path)
	require.NoError(t, os.Remove(path))
	wt.requireUnreadable(t)
	// Restored as it was: nothing to evaluate. A change of whitespace only
	// then shows neither the removal nor the restored file caused a restart.
	writeFile(t, path, content)
	wt.requireNoRestart(t, path)
	require.NoError(t, os.Remove(path))
	wt.requireUnreadable(t)
	writeFile(t, path, `{"client": {"insecure": true}}`)
	wt.requireRestart(t)
}

func TestWatchConfigYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "client:\n  insecure: false\n")
	wt := startWatcher(t, path)
	writeFile(t, path, "client:\n  insecure: true\n")
	wt.requireRestart(t)
}

// A config file which doesn't exist at start is watched for: once created
// with a config which differs from the defaults, the server restarts.
func TestWatchConfigFileCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	wt := startWatcher(t, path)
	writeFile(t, path, `{"client": {"insecure": true}}`)
	wt.requireRestart(t)
	require.Empty(t, wt.unreadable)
}
