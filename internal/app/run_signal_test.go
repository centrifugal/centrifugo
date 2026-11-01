//go:build !windows

package app

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// Signals are sent to the test process: once the first App started, the
// signal channel of run stays registered, so a signal arriving at any point
// of a restart is handled instead of ending the process.

// sendSignal sends sig to the test process and waits until it was
// delivered: to a channel of the test registered for it, and so to the
// channel of run.
func sendSignal(t *testing.T, sig syscall.Signal) {
	t.Helper()
	delivered := make(chan os.Signal, 1)
	signal.Notify(delivered, sig)
	defer signal.Stop(delivered)
	require.NoError(t, syscall.Kill(os.Getpid(), sig))
	select {
	case <-delivered:
	case <-time.After(watchWait):
		require.Fail(t, "signal not delivered")
	}
}

// SIGINT or SIGTERM at every phase of a restart stops the process: exit
// with the code of a stop by signal, pid file removed, no App served after.
func TestRunWatchConfigStopDuringRestart(t *testing.T) {
	for _, tc := range []struct {
		name string
		// ext returns an extension blocking the restart, closing entered
		// when the restart reached it, and what releases it.
		ext func() (ext Extension, logs *logRecorder, entered <-chan struct{}, release func())
		sig syscall.Signal
		// built is the number of Apps built.
		built int32
	}{
		{"shutdown", func() (Extension, *logRecorder, <-chan struct{}, func()) {
			e := &blockingStop{entered: make(chan struct{}), release: make(chan struct{})}
			return e, &e.logRecorder, e.entered, func() { close(e.release) }
		}, syscall.SIGTERM, 1},
		{"close", func() (Extension, *logRecorder, <-chan struct{}, func()) {
			e := &closeBlocker{entered: make(chan struct{}), release: make(chan struct{})}
			return e, &e.logRecorder, e.entered, func() { close(e.release) }
		}, syscall.SIGTERM, 1},
		{"New", func() (Extension, *logRecorder, <-chan struct{}, func()) {
			e := &setupBlocker{entered: make(chan struct{}), release: make(chan struct{})}
			return e, &e.logRecorder, e.entered, func() { close(e.release) }
		}, syscall.SIGTERM, 2},
		{"Start", func() (Extension, *logRecorder, <-chan struct{}, func()) {
			e := &startBlocker{entered: make(chan struct{})}
			return e, &e.logRecorder, e.entered, func() {}
		}, syscall.SIGINT, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer goleak.VerifyNone(t, leakOptions()...)

			ext, logs, entered, release := tc.ext()
			rt := runWatch(t, Options{Extensions: []Extension{ext}}, process{}, "")
			rt.next(t)
			rt.write(t, `, "client": {"insecure": true}`)
			requireClosed(t, entered, "restart did not reach the blocking extension")
			sendSignal(t, tc.sig)
			if tc.name == "New" {
				// Logged once New returned.
				require.Eventually(t, func() bool { return len(rt.signals) == 0 }, watchWait, watchTick)
			} else {
				require.Eventually(t, func() bool {
					return strings.Contains(logs.String(), "signal received: ")
				}, watchWait, watchTick)
			}
			release()
			rt.wait(t)
			require.Equal(t, []int{0}, rt.exitCodes())
			require.False(t, rt.pidFileExists())
			require.Equal(t, tc.built, rt.built.Load())
			require.Contains(t, logs.String(), "signal received: ")
		})
	}
}

// SIGTERM sent once a restart's Close returned, before the next App is
// built, stops the process.
func TestRunWatchConfigStopAfterRestartClose(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	closed := make(chan struct{}, 4)
	rt := runWatch(t, Options{}, process{closed: func() { closed <- struct{}{} }}, "")
	rt.next(t)
	rt.write(t, `, "client": {"insecure": true}`)
	requireClosed(t, closed, "App not closed")
	sendSignal(t, syscall.SIGTERM)
	rt.wait(t)
	require.Equal(t, []int{0}, rt.exitCodes())
	require.False(t, rt.pidFileExists())
}

// SIGTERM during New of a restarted App which doesn't return ends the
// process with code 1 once the backstop fired, removing the pid file.
func TestRunWatchConfigStopDuringRestartNewBackstop(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	ext := &setupBlocker{entered: make(chan struct{}), release: make(chan struct{})}
	rt := runWatch(t, Options{Extensions: []Extension{ext}}, process{backstop: 100 * time.Millisecond}, "")
	rt.next(t)
	rt.write(t, `, "client": {"insecure": true}`)
	requireClosed(t, ext.entered, "restart did not reach New")
	sendSignal(t, syscall.SIGTERM)
	require.Eventually(t, func() bool { return len(rt.exitCodes()) == 1 }, watchWait, watchTick)
	require.Equal(t, []int{1}, rt.exitCodes())
	require.False(t, rt.pidFileExists())
	close(ext.release)
	rt.wait(t)
	require.Equal(t, []int{1}, rt.exitCodes())
}

// SIGHUP arriving while a restarted App starts reloads its config once it
// runs; SIGHUP reaches the running App after a restart.
func TestRunWatchConfigReloadDuringRestart(t *testing.T) {
	defer goleak.VerifyNone(t, leakOptions()...)

	ext := &setupBlocker{entered: make(chan struct{}), release: make(chan struct{})}
	rt := runWatch(t, Options{Extensions: []Extension{ext}}, process{}, "")
	rt.next(t)
	rt.write(t, `, "client": {"insecure": true}`)
	requireClosed(t, ext.entered, "restart did not reach New")
	sendSignal(t, syscall.SIGHUP)
	require.Eventually(t, func() bool { return len(rt.signals) == 0 }, watchWait, watchTick)
	close(ext.release)
	rt.next(t)
	require.Eventually(t, func() bool {
		return strings.Count(ext.String(), "configuration successfully reloaded") == 1
	}, watchWait, watchTick)
	sendSignal(t, syscall.SIGHUP)
	require.Eventually(t, func() bool {
		return strings.Count(ext.String(), "configuration successfully reloaded") == 2
	}, watchWait, watchTick)
	sendSignal(t, syscall.SIGTERM)
	rt.wait(t)
	require.Equal(t, []int{0}, rt.exitCodes())
	require.False(t, rt.pidFileExists())
}
