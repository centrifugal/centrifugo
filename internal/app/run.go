package app

import (
	"errors"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// backstopMargin is how long after shutdown.timeout the backstop ends a
// shutdown which can't return. Shutdown, its steps left running or not,
// returns within shutdown.timeout, so the backstop never cuts it.
const backstopMargin = time.Second

// Run runs Centrifugo until it is asked to stop, then exits the process. On a
// restart (--watch-config-file) it shuts the App down and runs a new one, built
// from the config as it is then, in the same process. It is the only place
// which exits the process.
func Run(cmd *cobra.Command, configFile string, opts Options) {
	run(cmd, configFile, opts, process{exit: os.Exit})
}

// process is what run does with the process.
type process struct {
	// exit exits the process with a code.
	exit func(int)
	// backstop is the delay of the backstop after Shutdown began, or after
	// SIGINT or SIGTERM arrived while a restarted App starts; zero means
	// shutdown.timeout plus backstopMargin.
	backstop time.Duration
	// signals, when set, is the channel the process handles its signals on;
	// run creates one otherwise.
	signals chan os.Signal
	// closed, when set, is called once Close of an App returned, also when
	// the process was already exited because Close didn't return in time.
	closed func()
	// built, when set, is called with each App once New returned it.
	built func(a *App)
}

// runner runs the Apps of the process one after the other.
type runner struct {
	cmd        *cobra.Command
	configFile string
	opts       Options
	p          process
	// signals is registered by the first Start for the signals the App
	// handles and stays registered across restarts, so that no SIGINT or
	// SIGTERM gets its default action while a restart runs.
	signals chan os.Signal
	// environ is the environment the process started with, before .env was
	// loaded: each restart begins from it, so .env is loaded fresh.
	environ []string

	finishOnce sync.Once
	finished   atomic.Bool

	mu sync.Mutex
	// pidFile is the pid file written by the last App built, kept across
	// restarts.
	pidFile string
	// startBackstop bounds the startup of a restarted App which got SIGINT
	// or SIGTERM before it was built.
	startBackstop *time.Timer
}

// run is Run with what it does to the process; exit is called once.
func run(cmd *cobra.Command, configFile string, opts Options, p process) {
	r := &runner{
		cmd:        cmd,
		configFile: configFile,
		opts:       opts,
		p:          p,
		signals:    p.signals,
		environ:    os.Environ(),
	}
	if r.signals == nil {
		r.signals = make(chan os.Signal, 4)
	}
	defer signal.Stop(r.signals) // Once exit returned.
	var timeout time.Duration
	for restarted := false; ; restarted = true {
		if restarted {
			restoreEnviron(r.environ)
		}
		restart, a := r.runApp(restarted, timeout)
		if !restart {
			return
		}
		timeout = a.shutdownTimeout()
	}
}

// finish exits the process with code, once, removing the pid file when
// removePidFile is set.
func (r *runner) finish(removePidFile bool, code int) {
	r.finishOnce.Do(func() {
		r.finished.Store(true)
		r.stopStartBackstop()
		if removePidFile {
			r.mu.Lock()
			pidFile := r.pidFile
			r.mu.Unlock()
			if pidFile != "" {
				_ = os.Remove(pidFile)
			}
		}
		r.p.exit(code)
	})
}

func (r *runner) backstopDelay(timeout time.Duration) time.Duration {
	if r.p.backstop != 0 {
		return r.p.backstop
	}
	return timeout + backstopMargin
}

// armStartBackstop ends the process with code 1 when the startup of a
// restarted App, asked to stop before it was built, doesn't end within
// timeout, the shutdown.timeout of the App before it.
func (r *runner) armStartBackstop(timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.startBackstop != nil {
		return
	}
	r.startBackstop = time.AfterFunc(r.backstopDelay(timeout), func() {
		log.WithLevel(zerolog.FatalLevel).Msg("shutdown timeout reached")
		r.finish(true, 1)
	})
}

func (r *runner) stopStartBackstop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.startBackstop != nil {
		r.startBackstop.Stop()
	}
}

// runApp builds, starts and runs one App until it stops. It reports whether
// the process restarts, with the App, or was exited. timeout is the
// shutdown.timeout of the App before it on a restart.
func (r *runner) runApp(restarted bool, timeout time.Duration) (bool, *App) {
	p := r.p
	// Loaded before the baseline, so that it and the App see the same
	// environment. New reports an error loading it.
	_, _ = loadDotEnv()
	baseline := watchBaseline(r.cmd, r.configFile)

	// Before the first Start the signals aren't handled yet; afterwards they
	// are, and those arriving before Start returned are handled here.
	var starting *startSignals
	if restarted {
		starting = watchStartSignals(r.signals, func() { r.armStartBackstop(timeout) })
	}
	a, err := New(r.cmd, r.configFile, r.opts)
	if err != nil {
		if starting != nil {
			starting.end()
		}
		// New already logged the error and cleaned up. A restarted App
		// failing to build ends the process as a failed start does.
		r.finish(restarted, exitCode(err))
		return false, nil
	}
	if p.built != nil {
		p.built(a)
	}
	r.mu.Lock()
	if r.pidFile != "" && r.pidFile != a.cfg.PidFile {
		_ = os.Remove(r.pidFile)
	}
	r.pidFile = a.cfg.PidFile
	r.mu.Unlock()
	a.sigCh = r.signals
	if starting != nil {
		starting.setApp(a)
	}
	var reason ExitReason
	var code int
	err = a.Start()
	var stopped, reload bool
	if starting != nil {
		stopped, reload = starting.end()
	}
	switch {
	case stopped && (err == nil || errors.Is(err, errShutdownDuringStart)):
		reason = ExitStop
	case err != nil:
		reason, code = ExitRequested, exitCode(err) // Start already logged the error.
	default:
		if reload {
			a.reload()
		}
		restart := watchConfig(baseline, a.Lifecycle(), a.ConfigChangeRejected)
		reason, code = a.Wait(restart)
	}

	// A restart turns into a stop when SIGINT or SIGTERM arrived since its
	// shutdown began, or when its shutdown didn't finish in time: what was
	// left running would stay in the process. The pid file stop step was
	// skipped for the restart.
	restarting := func() bool {
		if reason != ExitRestart {
			return false
		}
		a.endStopWatch()
		return !a.stopRequestedDuringShutdown() && !a.shutdownTimedOut()
	}

	// timedOut ends a shutdown which didn't finish within shutdown.timeout.
	// A Close which didn't return in time is left running: the process exits
	// instead of building the next App, whose log writers the late Close
	// would replace with the ones from before it.
	timedOut := func() {
		event := log.WithLevel(zerolog.FatalLevel)
		if step := a.currentShutdownStep(); step != "" {
			event = event.Str("step", step)
		}
		event.Msg("shutdown timeout reached")
		a.endStopWatch()
		r.finish(true, 1)
	}
	shutdownTimeout := a.shutdownTimeout()
	// The backstop fires only if Shutdown itself can't return.
	stopEnd := time.Now().Add(shutdownTimeout)
	backstop := time.AfterFunc(r.backstopDelay(shutdownTimeout), timedOut)
	r.stopStartBackstop()
	code, _ = a.Shutdown(reason, code) // Errors are logged by Close.
	backstop.Stop()
	// Close gets what remains of shutdown.timeout.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		a.Close()
		if p.closed != nil {
			p.closed()
		}
	}()
	closeTimer := time.NewTimer(time.Until(stopEnd))
	defer closeTimer.Stop()
	select {
	case <-closed:
	case <-closeTimer.C:
		timedOut()
		return false, nil
	}
	if r.finished.Load() {
		return false, nil
	}
	if restarting() {
		return true, a
	}
	r.finish(reason == ExitRestart, code)
	return false, nil
}

// restoreEnviron makes the environment of the process environ: variables
// not in it are unset, the others set to their value in it.
func restoreEnviron(environ []string) {
	want := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v := splitEnv(kv)
		want[k] = v
	}
	for _, kv := range os.Environ() {
		k, _ := splitEnv(kv)
		if _, ok := want[k]; !ok {
			_ = os.Unsetenv(k)
		}
	}
	for k, v := range want {
		if cur, ok := os.LookupEnv(k); !ok || cur != v {
			_ = os.Setenv(k, v)
		}
	}
}

// splitEnv splits an environment entry "key=value". The key is not empty:
// on Windows some begin with "=".
func splitEnv(kv string) (key string, value string) {
	if len(kv) == 0 {
		return "", ""
	}
	if i := strings.IndexByte(kv[1:], '='); i >= 0 {
		return kv[:i+1], kv[i+2:]
	}
	return kv, ""
}

// startSignals handles the signals of a restarted App until Start returned:
// SIGINT and SIGTERM stop the App, which once built is quiesced so that
// Start returns early; SIGHUP is kept for after Start. Signals received
// before the App was built are logged once it is: New sets up the logger.
type startSignals struct {
	signals <-chan os.Signal
	done    chan struct{}
	exited  chan struct{}

	mu      sync.Mutex
	app     *App
	built   bool // New returned
	pending []os.Signal
	stop    bool
	reload  bool
	// unbuilt is called once when SIGINT or SIGTERM arrives before the App
	// was built.
	unbuilt func()
}

func watchStartSignals(signals <-chan os.Signal, unbuilt func()) *startSignals {
	s := &startSignals{signals: signals, done: make(chan struct{}), exited: make(chan struct{}), unbuilt: unbuilt}
	go func() {
		defer close(s.exited)
		for {
			select {
			case sig := <-signals:
				s.received(sig)
			case <-s.done:
				return
			}
		}
	}()
	return s
}

func (s *startSignals) received(sig os.Signal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.built {
		logSignal(sig)
	} else {
		s.pending = append(s.pending, sig)
	}
	switch sig {
	case syscall.SIGHUP:
		s.reload = true
	case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
		s.stop = true
		if s.app != nil {
			s.app.lifecycle.quiesce()
		} else if s.unbuilt != nil {
			s.unbuilt()
			s.unbuilt = nil
		}
	}
}

func logSignal(sig os.Signal) {
	log.Info().Msgf("signal received: %v", sig)
}

// newReturned logs the signals received while New ran. Called once New
// returned.
func (s *startSignals) newReturned() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.built = true
	for _, sig := range s.pending {
		logSignal(sig)
	}
	s.pending = nil
}

// setApp sets the App built, quiescing it when a stop was received.
func (s *startSignals) setApp(a *App) {
	s.newReturned()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.app = a
	if s.stop {
		a.lifecycle.quiesce()
	}
}

// end stops handling the signals, handling those received but not read yet,
// and reports whether a stop and a reload were received.
func (s *startSignals) end() (stop bool, reload bool) {
	s.newReturned()
	close(s.done)
	<-s.exited
	for drained := false; !drained; {
		select {
		case sig := <-s.signals:
			s.received(sig)
		default:
			drained = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stop, s.reload
}
