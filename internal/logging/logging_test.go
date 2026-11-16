package logging

import (
	"bytes"
	"io"
	"path/filepath"
	"sync"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func setupTo(t *testing.T, w io.Writer) func() {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Log.File = filepath.Join(t.TempDir(), "log")
	_, closeFn, err := Setup(cfg, Options{Writers: []io.Writer{w}})
	require.NoError(t, err)
	return closeFn
}

// A Setup and a close while other goroutines log (several apps in one
// process) do not race on the global logger (run with -race).
func TestSetupWhileLogging(t *testing.T) {
	closeFirst := setupTo(t, io.Discard)
	defer closeFirst()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				log.Info().Msg("logging")
			}
		}
	}()
	for i := 0; i < 10; i++ {
		setupTo(t, io.Discard)()
	}
	close(stop)
	wg.Wait()
}

// The latest Setup's writers get the lines; its close gives them back to
// the previous Setup's.
func TestSetupCloseRestoresWriter(t *testing.T) {
	first, second := &syncBuffer{}, &syncBuffer{}
	closeFirst := setupTo(t, first)
	defer closeFirst()

	closeSecond := setupTo(t, second)
	log.Info().Msg("to second")
	closeSecond()
	log.Info().Msg("to first")

	require.Contains(t, second.String(), "to second")
	require.NotContains(t, second.String(), "to first")
	require.Contains(t, first.String(), "to first")
	require.NotContains(t, first.String(), "to second")
}
