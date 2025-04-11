package redisqueue

import (
	"os"
	"os/signal"
	"syscall"
)

// newSignalHandler registered for SIGTERM and SIGINT. A stop channel is
// returned which is closed on one of these signals. If a second signal is
// caught, the program is terminated with exit code 1.
func newSignalHandler() <-chan struct{} {
	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1)
	}()

	return stop
}
