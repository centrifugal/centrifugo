package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// watchConfigInterval is how often the config file is read.
const watchConfigInterval = 250 * time.Millisecond

// errConfigFileNotFound is returned when the config file disappeared between
// reading it and loading the config from it.
var errConfigFileNotFound = errors.New("config file not found")

// watchConfigEnabled reports whether this process watches the config file:
// the command has --watch-config-file set.
func watchConfigEnabled(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	enabled, err := cmd.Flags().GetBool("watch-config-file")
	return err == nil && enabled
}

// configWatcher polls the config file and signals a restart once the file
// holds a valid config which differs from the running one.
type configWatcher struct {
	cmd        *cobra.Command
	configFile string
	interval   time.Duration

	// lastBytes are the file contents last evaluated (or the baseline);
	// readable is false while the file can't be read.
	lastBytes []byte
	readable  bool
	// hash is the hash of the effective config of the running server.
	hash [32]byte

	// onRejected is called with the error of a changed config which fails
	// the checks; onUnchanged when a changed file has the same effective
	// config; onUnreadable when the file becomes unreadable. They default to
	// logging.
	onRejected   func(err error)
	onUnchanged  func()
	onUnreadable func(err error)
}

// watchBaseline returns a watcher holding the current config file contents
// and effective config as its baseline, nil without --watch-config-file. Taken
// before the server is built, so that an edit made while it starts is seen
// as a change.
func watchBaseline(cmd *cobra.Command, configFile string) *configWatcher {
	if !watchConfigEnabled(cmd) {
		return nil
	}
	return newConfigWatcher(cmd, configFile)
}

// newConfigWatcher returns a watcher with the current contents of
// configFile and its effective config as the baseline. When the config
// can't be loaded, any later valid config is a change.
func newConfigWatcher(cmd *cobra.Command, configFile string) *configWatcher {
	w := &configWatcher{
		cmd:        cmd,
		configFile: configFile,
		interval:   watchConfigInterval,
	}
	data, err := os.ReadFile(configFile) //nolint:gosec // The config file given by the user.
	if err == nil {
		w.lastBytes = data
		w.readable = true
	}
	if hash, err := w.effectiveConfigHash(); err == nil {
		w.hash = hash
	}
	return w
}

// effectiveConfigHash loads the config the way the server does on start,
// checks it and returns the hash of its JSON encoding.
func (w *configWatcher) effectiveConfigHash() ([32]byte, error) {
	cfg, meta, err := config.GetConfig(w.cmd, w.configFile)
	if err != nil {
		return [32]byte{}, err
	}
	if meta.FileNotFound {
		return [32]byte{}, errConfigFileNotFound
	}
	if err := cfg.Validate(); err != nil {
		return [32]byte{}, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}

func (w *configWatcher) rejected(err error) {
	if w.onRejected != nil {
		w.onRejected(err)
		return
	}
	log.Error().Err(err).Msg("config file change rejected, keep running with current config")
}

func (w *configWatcher) unreadable(err error) {
	if w.onUnreadable != nil {
		w.onUnreadable(err)
		return
	}
	log.Warn().Err(err).Msg("config file is not readable, keep running with current config")
}

func (w *configWatcher) unchanged() {
	if w.onUnchanged != nil {
		w.onUnchanged()
		return
	}
	log.Debug().Msg("config file changed, no effective changes")
}

// run polls the config file until ctx is done or the file holds a valid
// config which differs from the baseline. Then it sends to restart once and
// returns, with that config as its new baseline.
//
// Changed contents are evaluated once a second read returns the same
// contents, so a file being written is not evaluated half-way.
func (w *configWatcher) run(ctx context.Context, restart chan<- struct{}) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	var pending []byte
	hasPending := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		data, err := os.ReadFile(w.configFile)
		if err != nil {
			hasPending = false
			if w.readable {
				w.readable = false
				w.unreadable(err)
			}
			continue
		}
		w.readable = true
		if bytes.Equal(data, w.lastBytes) {
			hasPending = false
			continue
		}
		if !hasPending || !bytes.Equal(data, pending) {
			pending, hasPending = data, true
			continue
		}
		hasPending = false
		hash, err := w.effectiveConfigHash()
		if errors.Is(err, errConfigFileNotFound) {
			continue // Removed since read: evaluated once it's back.
		}
		w.lastBytes = data
		if err != nil {
			w.rejected(err)
			continue
		}
		if hash == w.hash {
			w.unchanged()
			continue
		}
		w.hash = hash
		log.Info().Msg("config file changed, restarting")
		select {
		case restart <- struct{}{}:
		case <-ctx.Done():
		}
		return
	}
}

// watchConfig starts w as a producer of lc and returns the channel it
// signals a restart on; nil when w is nil, which never fires. Rejected
// changes go to rejected.
func watchConfig(w *configWatcher, lc *Lifecycle, rejected func(err error)) <-chan struct{} {
	if w == nil {
		return nil
	}
	w.onRejected = rejected
	restart := make(chan struct{}, 1)
	lc.Go("config watcher", func(ctx context.Context) {
		w.run(ctx, restart)
	})
	return restart
}
