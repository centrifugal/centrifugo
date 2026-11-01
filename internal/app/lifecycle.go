package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/client"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/service"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// maxStopGrace bounds the part of shutdown.timeout kept for the stop steps
// which remain once the shutdown deadline has passed, and for Close.
const maxStopGrace = 5 * time.Second

// stopGrace is the part of timeout kept after the shutdown deadline: a fifth
// of it, at most maxStopGrace. The stop steps which remain once the deadline
// passed share it, except its last fifth, kept for Close.
func stopGrace(timeout time.Duration) (grace time.Duration, closeBudget time.Duration) {
	grace = min(maxStopGrace, timeout/5)
	return grace, grace / 5
}

// handledSignals are the signals the App handles once it runs.
var handledSignals = []os.Signal{syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM}

// defaultShutdownTimeout bounds shutdown when the config could not be loaded.
const defaultShutdownTimeout = 30 * time.Second

// Lifecycle lets components and extensions register what must stop with the
// App, run producer loops and ask the App to exit.
type Lifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	stopping  bool
	steps     []stopStep
	wg        sync.WaitGroup
	producers map[*producer]struct{}

	exitOnce sync.Once
	exitCh   chan struct{}
	exitCode int
	exitErr  error
	// exitDuringShutdown is set when Exit was first called after shutdown began.
	exitDuringShutdown bool
}

type stopStep struct {
	name string
	stop func(ctx context.Context) error
}

// producer is a loop started with Lifecycle.Go.
type producer struct {
	name        string
	goroutineID uint64
}

func newLifecycle() *Lifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifecycle{
		ctx:       ctx,
		cancel:    cancel,
		exitCh:    make(chan struct{}),
		producers: map[*producer]struct{}{},
	}
}

// OnStop registers a stop step. Stop steps run one at a time in reverse
// registration order at the end of Shutdown, each with a context bounded by
// the shutdown deadline (expired once it has passed: a step then still closes
// what it owns, within the grace the remaining steps share). A stop step must be idempotent, safe on a
// half-built object, close what its component owns and wait for the
// goroutines it started. Registered after shutdown began, the step runs at once.
func (l *Lifecycle) OnStop(name string, stop func(ctx context.Context) error) {
	l.mu.Lock()
	if l.stopping {
		l.mu.Unlock()
		if err := stop(context.Background()); err != nil {
			log.Error().Err(err).Str("step", name).Msg("error stopping")
		}
		return
	}
	l.steps = append(l.steps, stopStep{name: name, stop: stop})
	l.mu.Unlock()
}

// Go runs a producer loop: f's ctx is cancelled when shutdown begins, and
// Shutdown waits for f to return before the node stops. Refused (false) once
// shutdown began. Not for loops the node needs until it stops, and never for
// per-connection, per-command or per-publication work.
func (l *Lifecycle) Go(name string, f func(ctx context.Context)) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopping {
		return false
	}
	p := &producer{name: name}
	l.producers[p] = struct{}{}
	l.wg.Add(1)
	go func() {
		l.mu.Lock()
		p.goroutineID = currentGoroutineID()
		l.mu.Unlock()
		defer func() {
			l.mu.Lock()
			delete(l.producers, p)
			l.mu.Unlock()
			l.wg.Done()
		}()
		f(l.ctx)
	}()
	return true
}

// Context is cancelled when shutdown begins.
func (l *Lifecycle) Context() context.Context {
	return l.ctx
}

// Exit asks the App to stop with a code instead of exiting the process. The
// first call wins. During New, New returns an error; during Start, Start
// returns one; afterwards Wait returns ExitRequested with the code; during
// Shutdown it sets the final code.
func (l *Lifecycle) Exit(code int, err error) {
	l.exitOnce.Do(func() {
		l.mu.Lock()
		l.exitCode = code
		l.exitErr = err
		l.exitDuringShutdown = l.stopping
		l.mu.Unlock()
		close(l.exitCh)
		if l.exitDuringShutdown {
			logExitRequested(&ExitError{Code: code, Err: err})
		}
	})
}

// exited returns the ExitError of an Exit call, nil when Exit was not called.
func (l *Lifecycle) exited() *ExitError {
	select {
	case <-l.exitCh:
		l.mu.Lock()
		defer l.mu.Unlock()
		return &ExitError{Code: l.exitCode, Err: l.exitErr}
	default:
		return nil
	}
}

// isStopping reports whether shutdown began.
func (l *Lifecycle) isStopping() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stopping
}

// quiesce cancels Context and refuses new producers.
func (l *Lifecycle) quiesce() {
	l.mu.Lock()
	l.stopping = true
	l.mu.Unlock()
	l.cancel()
}

// runningProducers returns the names and goroutines of producers which have
// not returned.
func (l *Lifecycle) runningProducers() ([]string, []uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var names []string
	var ids []uint64
	for p := range l.producers {
		names = append(names, p.name)
		if p.goroutineID != 0 {
			ids = append(ids, p.goroutineID)
		}
	}
	slices.Sort(names)
	return names, ids
}

// takeStopSteps returns the registered stop steps and refuses new ones: a
// step registered afterwards runs at once.
func (l *Lifecycle) takeStopSteps() []stopStep {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopping = true
	steps := l.steps
	l.steps = nil
	return steps
}

// ExitReason is why Wait returned.
type ExitReason int

const (
	// ExitStop means the process was asked to stop (SIGINT or SIGTERM).
	ExitStop ExitReason = iota
	// ExitRestart means the restart channel given to Wait fired.
	ExitRestart
	// ExitRequested means Lifecycle.Exit was called.
	ExitRequested
)

// ExitError is returned by New and Start when Lifecycle.Exit was called
// during them.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit requested with code %d", e.Code)
	}
	return fmt.Sprintf("exit requested with code %d: %v", e.Code, e.Err)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// exitCode is the process exit code for an error returned by New or Start.
func exitCode(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}

// errShutdownDuringStart is returned by Start when Shutdown began while it ran.
var errShutdownDuringStart = errors.New("shutdown began during start")

// Start binds every listener, then runs the node, services and consumers,
// registers signal handling (on the channel Run set, which Run keeps
// registered across restarts) and runs the servers. On error it returns it (already
// logged); the caller then runs Shutdown and Close.
func (a *App) Start() error {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	if a.lifecycle.isStopping() {
		return errShutdownDuringStart
	}
	cfg := a.cfg

	a.muxCtx = &MuxContext{Config: cfg, Container: a.container, Node: a.node, Lifecycle: a.lifecycle}
	err := eachExtension(a.extensions, func(name string, e MuxExtender) error {
		return wrapExtensionError(name, e.ExtendMux(a.muxCtx))
	})
	a.muxCtx.sealed = true
	if err != nil {
		return a.fatal(err, "error extending HTTP server")
	}
	if exitErr := a.lifecycle.exited(); exitErr != nil {
		return a.exitRequested(exitErr)
	}

	// Build the servers and bind every listener before anything runs.
	if err := a.buildHTTPServers(); err != nil {
		return a.fatal(err, "error running HTTP server")
	}
	if cfg.GrpcAPI.Enabled {
		a.grpcAPIServer, err = a.buildGRPCAPIServer()
		if err != nil {
			return a.fatal(err, "error creating GRPC API server")
		}
	}
	if cfg.UniGRPC.Enabled {
		a.grpcUniServer, err = a.buildGRPCUniServer()
		if err != nil {
			return a.fatal(err, "error creating uni GRPC server")
		}
	}
	if a.lifecycle.isStopping() {
		return errShutdownDuringStart
	}

	if err := a.node.Run(); err != nil {
		return a.fatal(err, "error running node")
	}
	a.mu.Lock()
	a.nodeRunning = true
	a.mu.Unlock()
	if a.lifecycle.isStopping() {
		return errShutdownDuringStart
	}

	a.serviceDone = a.runServices(a.serviceCtx, a.services)
	a.ingestDone = a.runServices(a.ingestCtx, a.ingest)

	// Signals are handled from before the servers accept connections: one
	// arriving before Start returned is handled by Wait.
	if a.sigCh == nil {
		a.sigCh = make(chan os.Signal, 1)
		a.ownSignals = true
	}
	signal.Notify(a.sigCh, handledSignals...)

	a.serve()

	logStartWarnings(cfg, a.cfgMeta)

	if exitErr := a.lifecycle.exited(); exitErr != nil {
		return a.exitRequested(exitErr)
	}
	return nil
}

// Addrs returns the addresses of the listeners bound by Start.
func (a *App) Addrs() []net.Addr {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]net.Addr(nil), a.addrs...)
}

func (a *App) addAddr(addr net.Addr) {
	a.mu.Lock()
	a.addrs = append(a.addrs, addr)
	a.mu.Unlock()
}

// Wait handles signals until the App is asked to stop: SIGHUP reloads the
// config, SIGINT and SIGTERM return ExitStop. It also returns ExitRestart when
// restart fires (nil never does) and ExitRequested with the code of
// Lifecycle.Exit.
func (a *App) Wait(restart <-chan struct{}) (ExitReason, int) {
	for {
		select {
		case sig := <-a.sigCh:
			log.Info().Msgf("signal received: %v", sig)
			switch sig {
			case syscall.SIGHUP:
				a.reload()
			case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
				return ExitStop, 0
			}
		case <-restart:
			return ExitRestart, 0
		case <-a.lifecycle.exitCh:
			exitErr := a.lifecycle.exited()
			return ExitRequested, exitErr.Code
		}
	}
}

// reload reloads configuration on SIGHUP. Centrifugo can't reload config for
// everything – just best effort to reload what's possible: channel options and
// token verifiers.
func (a *App) reload() {
	log.Info().Msg("reloading configuration")
	err := a.reloadFromFile()
	_ = eachExtension(a.extensions, func(_ string, o ConfigObserver) error {
		o.ConfigReloaded(err)
		return nil
	})
	if err == nil {
		log.Info().Msg("configuration successfully reloaded")
	}
}

func (a *App) reloadFromFile() error {
	newCfg, _, err := config.GetConfig(a.cmd, a.configFile)
	if err != nil {
		log.Err(err).Msg("error reading config")
		return err
	}
	if err = newCfg.Validate(); err != nil {
		log.Error().Msgf("error validating config: %v", err)
		return err
	}
	if err = reloadConfig(newCfg, a.container, a.proxyMap, a.tokenVerifier, a.subTokenVerifier); err != nil {
		log.Error().Msgf("error reloading: %v", err)
		return err
	}
	return nil
}

// ConfigChangeRejected logs a rejected config file change and passes it to
// ConfigObserver extensions.
func (a *App) ConfigChangeRejected(err error) {
	log.Error().Err(err).Msg("config file change rejected, keep running with current config")
	_ = eachExtension(a.extensions, func(_ string, o ConfigObserver) error {
		o.ConfigChangeRejected(err)
		return nil
	})
}

// shutdownTimeout is shutdown.timeout of the config, or its default when the
// config wasn't loaded.
func (a *App) shutdownTimeout() time.Duration {
	if t := a.cfg.Shutdown.Timeout.ToDuration(); t > 0 {
		return t
	}
	return defaultShutdownTimeout
}

// Shutdown stops the App within shutdown.timeout: producers and consumers
// stop taking new work, servers stop accepting, consumers drain, producers
// are waited for, the node shuts down, servers finish, services stop, then the
// stop steps run in reverse registration order. The steps share
// shutdown.timeout minus its grace (a fifth, at most 5 seconds); the stop
// steps remaining once that deadline passed share most of the grace, the
// rest is kept for Close. It returns the final exit code and every error,
// joined. Calls after the first return the same result.
func (a *App) Shutdown(reason ExitReason, code int) (int, error) {
	a.shutdownOnce.Do(func() {
		a.shutdownCode, a.shutdownErr = a.shutdown(reason, code)
	})
	return a.shutdownCode, a.shutdownErr
}

func (a *App) shutdown(reason ExitReason, code int) (int, error) {
	log.Info().Msg("shutting down ...")
	a.mu.Lock()
	a.shutdownReason = reason
	if reason == ExitRestart && a.sigCh != nil && !a.stopWatchEnded {
		a.stopWatchDone = make(chan struct{})
		a.stopWatchExited = make(chan struct{})
		go a.watchStopDuringShutdown(a.stopWatchDone, a.stopWatchExited)
	}
	a.mu.Unlock()

	s := newShutdownRun(a.shutdownTimeout(), &a.shutdownStep)
	defer s.cancel()
	var errs []error

	// 1. Quiesce: producers and long-running requests stop taking new work. A
	// Start in progress returns at its next step.
	a.lifecycle.quiesce()
	_ = s.step("start", nil, func(context.Context) error {
		a.startMu.Lock()
		a.startMu.Unlock() //nolint:staticcheck // Waits for Start to return.
		return nil
	})

	// 2. Servers stop accepting. Not awaited here: streaming connections end
	// only when the node disconnects their clients.
	serversDone, serverGoroutines := a.stopServers(s.ctx)

	// 3. Consumers drain. They are stopped before the node, so that no message
	// is acknowledged after this node lost the ability to deliver it.
	a.ingestCancel()
	if err := a.closeConsumers(s); err != nil {
		errs = append(errs, err)
	}
	if a.ingestDone != nil {
		s.waitChan("consumers", a.ingestDone, nil)
	}

	// 4. Producers.
	_ = s.step("producers", a.lifecycle.runningProducers, func(context.Context) error {
		a.lifecycle.wg.Wait()
		return nil
	})

	// 5. Node: disconnects clients and closes the engines set on it.
	a.mu.Lock()
	nodeRunning := a.nodeRunning
	a.mu.Unlock()
	if nodeRunning {
		if err := s.step("node", nil, func(ctx context.Context) error {
			if err := a.node.Shutdown(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return nil
		}); err != nil {
			errs = append(errs, err)
		}
	}

	// 6. Servers finish; when they don't in time they are closed.
	if !s.waitChan("servers", serversDone, func() ([]string, []uint64) { return nil, serverGoroutines() }) {
		_ = s.step("servers", nil, func(context.Context) error {
			a.closeServers()
			return nil
		})
	}

	// 7. Services.
	a.serviceCancel()
	if a.serviceDone != nil {
		s.waitChan("services", a.serviceDone, nil)
	}

	// 8. Stop steps, one at a time in reverse registration order.
	steps := a.lifecycle.takeStopSteps()
	for i := len(steps) - 1; i >= 0; i-- {
		if err := s.step(steps[i].name, nil, steps[i].stop); err != nil {
			errs = append(errs, err)
		}
	}

	a.shutdownStep.Store("")

	// 9. Result.
	if s.timedOut {
		log.WithLevel(zerolog.FatalLevel).Msg("shutdown timeout reached")
		code = 1
		a.timedOut.Store(true)
	}
	if exitErr := a.lifecycle.exited(); exitErr != nil && reason != ExitRequested {
		code = exitErr.Code
	}
	return code, errors.Join(errs...)
}

// closeConsumers drains every consumer concurrently through Close.
func (a *App) closeConsumers(s *shutdownRun) error {
	if len(a.consumers) == 0 {
		return nil
	}
	var mu sync.Mutex
	var errs []error
	var ids []uint64
	var wg sync.WaitGroup
	done := make(chan struct{})
	for _, c := range a.consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			ids = append(ids, currentGoroutineID())
			mu.Unlock()
			if err := c.Close(s.ctx); err != nil && !errors.Is(err, context.Canceled) {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	s.waitChan("consumers", done, func() ([]string, []uint64) {
		mu.Lock()
		defer mu.Unlock()
		return nil, append([]uint64(nil), ids...)
	})
	mu.Lock()
	defer mu.Unlock()
	if len(errs) > 0 {
		return fmt.Errorf("consumers: %w", errors.Join(errs...))
	}
	return nil
}

// shutdownRun bounds the steps of one shutdown by its deadline, then by the
// grace the steps remaining after the deadline share. Both fit in the
// shutdown timeout, with the part of the grace kept for Close left over.
type shutdownRun struct {
	ctx      context.Context
	cancel   context.CancelFunc
	deadline time.Time
	grace    time.Time
	timedOut bool
	// current holds the name of the step running.
	current *atomic.Value
}

func newShutdownRun(timeout time.Duration, current *atomic.Value) *shutdownRun {
	start := time.Now()
	grace, closeBudget := stopGrace(timeout)
	deadline := start.Add(timeout - grace)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	return &shutdownRun{ctx: ctx, cancel: cancel, deadline: deadline, grace: start.Add(timeout - closeBudget), current: current}
}

// bound returns how long a step may still be waited for.
func (s *shutdownRun) bound() time.Duration {
	if time.Now().Before(s.deadline) {
		return time.Until(s.deadline)
	}
	s.timedOut = true
	return time.Until(s.grace)
}

// step runs f in its own goroutine with the shutdown context and waits for
// it within the bound. A step which doesn't return in time is logged by name,
// with the stacks of its goroutine and of the goroutines owned returns, and
// left running. Errors are returned prefixed with the step name.
func (s *shutdownRun) step(name string, owned func() ([]string, []uint64), f func(ctx context.Context) error) error {
	s.current.Store(name)
	errCh := make(chan error, 1)
	idCh := make(chan uint64, 1)
	go func() {
		idCh <- currentGoroutineID()
		errCh <- f(s.ctx)
	}()
	id := <-idCh
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	case <-time.After(s.bound()):
		s.timedOut = true
		var names []string
		ids := []uint64{id}
		if owned != nil {
			n, more := owned()
			names = n
			ids = append(ids, more...)
		}
		logHungStep(name, names, ids)
		return nil
	}
}

// waitChan waits for ch to be closed within the bound and reports whether it
// was. When it wasn't, the step is logged as step does.
func (s *shutdownRun) waitChan(name string, ch <-chan struct{}, owned func() ([]string, []uint64)) bool {
	s.current.Store(name)
	select {
	case <-ch:
		return true
	default:
	}
	select {
	case <-ch:
		return true
	case <-time.After(s.bound()):
		s.timedOut = true
		var names []string
		var ids []uint64
		if owned != nil {
			names, ids = owned()
		}
		logHungStep(name, names, ids)
		return false
	}
}

func logHungStep(name string, names []string, ids []uint64) {
	event := log.Error().Str("step", name)
	if len(names) > 0 {
		event = event.Strs("running", names)
	}
	if stacks := goroutineStacks(ids); stacks != "" {
		event = event.Str("stacks", stacks)
	}
	event.Msg("shutdown step did not finish in time, leaving it running")
}

// currentGoroutineID returns the ID of the calling goroutine.
func currentGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// "goroutine 123 [running]: ..."
	b := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i > 0 {
		id, _ := strconv.ParseUint(string(b[:i]), 10, 64)
		return id
	}
	return 0
}

// goroutineStacks returns the stacks of the goroutines with ids which are
// still running.
func goroutineStacks(ids []uint64) string {
	if len(ids) == 0 {
		return ""
	}
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	var out bytes.Buffer
	for _, g := range bytes.Split(buf, []byte("\n\n")) {
		b := bytes.TrimPrefix(g, []byte("goroutine "))
		i := bytes.IndexByte(b, ' ')
		if i <= 0 {
			continue
		}
		id, err := strconv.ParseUint(string(b[:i]), 10, 64)
		if err != nil || !slices.Contains(ids, id) {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.Write(g)
	}
	return out.String()
}

// currentShutdownStep returns the name of the shutdown step running, empty
// when Shutdown doesn't run.
func (a *App) currentShutdownStep() string {
	name, _ := a.shutdownStep.Load().(string)
	return name
}

// watchStopDuringShutdown records SIGINT and SIGTERM arriving from the start
// of a restart's shutdown until endStopWatch.
func (a *App) watchStopDuringShutdown(done <-chan struct{}, exited chan<- struct{}) {
	defer close(exited)
	for {
		select {
		case sig := <-a.sigCh:
			a.recordStopDuringShutdown(sig)
		case <-done:
			return
		}
	}
}

func (a *App) recordStopDuringShutdown(sig os.Signal) {
	log.Info().Msgf("signal received: %v", sig)
	switch sig {
	case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
		a.stopRequested.Store(true)
	}
}

// endStopWatch ends the watching of a restart's shutdown, recording the
// signals received but not read yet. Signal handling stays registered, so a
// signal arriving afterwards doesn't end the process. Run calls it once the
// App closed, before it runs the next App or exits the process.
func (a *App) endStopWatch() {
	a.stopWatchOnce.Do(func() {
		a.mu.Lock()
		a.stopWatchEnded = true
		done, exited := a.stopWatchDone, a.stopWatchExited
		a.mu.Unlock()
		if done == nil {
			return
		}
		close(done)
		<-exited
		for {
			select {
			case sig := <-a.sigCh:
				a.recordStopDuringShutdown(sig)
			default:
				return
			}
		}
	})
}

// stopSignals stops signal handling on the channel Start created. The
// channel Run set stays registered: Run stops it.
func (a *App) stopSignals() {
	if a.sigCh != nil && a.ownSignals {
		signal.Stop(a.sigCh)
	}
}

// stopRequestedDuringShutdown reports whether SIGINT or SIGTERM arrived while
// the shutdown of a restart ran.
func (a *App) stopRequestedDuringShutdown() bool {
	return a.stopRequested.Load()
}

// shutdownTimedOut reports whether a step of Shutdown didn't finish within
// shutdown.timeout.
func (a *App) shutdownTimedOut() bool {
	return a.timedOut.Load()
}

// Close runs the close tier: it logs the errors of Shutdown, stops signal
// handling (unless Run set the channel, which stays registered so that no
// SIGINT or SIGTERM ends the process before Run exited it or ran the next
// App), closes log writers, then the log file and handler, restoring the
// global logger. It runs Shutdown first if it hasn't run.
func (a *App) Close() {
	a.closeOnce.Do(func() {
		_, err := a.Shutdown(ExitStop, 0)
		if err != nil {
			log.Error().Err(err).Msg("error shutting down")
		}
		a.closeTier()
	})
}

// closeTier stops signal handling, then closes log writers and the log
// file.
func (a *App) closeTier() {
	a.stopSignals()
	for i := len(a.logClosers) - 1; i >= 0; i-- {
		a.logClosers[i]()
	}
	a.logClosers = nil
}

// Lifecycle returns the App's Lifecycle.
func (a *App) Lifecycle() *Lifecycle {
	return a.lifecycle
}

// runServices runs all services registered in the manager and returns a channel
// closed when all of them stopped. A service which fails asks the App to exit
// with code 1.
func (a *App) runServices(ctx context.Context, manager *service.Manager) chan struct{} {
	done := make(chan struct{})
	manager.Run(ctx)
	go func() {
		defer close(done)
		err := manager.Wait()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("error running service")
			a.lifecycle.Exit(1, err)
		}
	}()
	return done
}

// reloadConfig applies newCfg to the token verifiers and the config container.
// Everything which can fail is done before any verifier changes, so the new
// config is applied either fully or not at all. Verifier parts replaced by the
// reload, or prepared and not applied, are closed.
func reloadConfig(
	newCfg config.Config, cfgContainer *config.Container, proxyMap *client.ProxyMap,
	tokenVerifier *jwtverify.VerifierJWT, subTokenVerifier *jwtverify.VerifierJWT,
) (err error) {
	if err := checkProxiesBuilt(newCfg, proxyMap); err != nil {
		return err
	}
	verifierConfig, err := confighelpers.MakeVerifierConfig(newCfg.Client.Token)
	if err != nil {
		return err
	}
	applyTokenVerifier, discardTokenVerifier, err := tokenVerifier.PrepareReload(verifierConfig)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			discardTokenVerifier()
		}
	}()
	applySubTokenVerifier := func() {}
	if subTokenVerifier != nil {
		var subVerifierConfig jwtverify.VerifierConfig
		subVerifierConfig, err = confighelpers.MakeVerifierConfig(newCfg.Client.SubscriptionToken.Token)
		if err != nil {
			return err
		}
		var discardSubTokenVerifier func()
		applySubTokenVerifier, discardSubTokenVerifier, err = subTokenVerifier.PrepareReload(subVerifierConfig)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				discardSubTokenVerifier()
			}
		}()
	}
	if err = cfgContainer.Reload(newCfg); err != nil {
		return err
	}
	applyTokenVerifier()
	applySubTokenVerifier()
	return nil
}

// serve starts serving on the listeners bound by Start.
func (a *App) serve() {
	a.serveWG = &sync.WaitGroup{}
	for _, g := range []*grpcServer{a.grpcAPIServer, a.grpcUniServer} {
		if g == nil {
			continue
		}
		log.Info().Msg(g.logLine)
		a.serveWG.Add(1)
		go func() {
			defer a.serveWG.Done()
			if err := g.server.Serve(g.ln); err != nil {
				a.serveError(err, g.errMsg)
			}
		}()
	}
	a.serveACME(a.serveWG)
	for _, hs := range a.httpServers {
		a.serveHTTP(hs, a.serveWG)
	}
}

// stopServers makes the servers stop accepting: gRPC GracefulStop and HTTP
// Shutdown (bounded by ctx) run in goroutines. The returned channel is closed
// once they and the serving goroutines returned; the function returns the
// goroutines stopping the servers.
func (a *App) stopServers(ctx context.Context) (<-chan struct{}, func() []uint64) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ids []uint64
	start := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			ids = append(ids, currentGoroutineID())
			mu.Unlock()
			f()
		}()
	}
	for _, g := range []*grpcServer{a.grpcAPIServer, a.grpcUniServer} {
		if g != nil {
			start(g.server.GracefulStop)
		}
	}
	for _, hs := range a.httpServers {
		start(func() { _ = hs.server.Shutdown(ctx) })
	}
	if a.acmeServer != nil {
		start(func() { _ = a.acmeServer.server.Shutdown(ctx) })
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		if a.serveWG != nil {
			a.serveWG.Wait()
		}
		close(done)
	}()
	return done, func() []uint64 {
		mu.Lock()
		defer mu.Unlock()
		return append([]uint64(nil), ids...)
	}
}

// closeServers closes the servers which didn't finish in time.
func (a *App) closeServers() {
	for _, g := range []*grpcServer{a.grpcAPIServer, a.grpcUniServer} {
		if g != nil {
			g.server.Stop()
		}
	}
	for _, hs := range a.httpServers {
		_ = hs.server.Close()
		hs.closeHTTP3()
	}
	if a.acmeServer != nil {
		_ = a.acmeServer.server.Close()
	}
}
