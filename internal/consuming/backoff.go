package consuming

import (
	"math/rand"
	"time"
)

const (
	minBackoffDuration = 10 * time.Millisecond
	maxBackoffDuration = 5 * time.Second
)

func getNextBackoffDuration(currentBackoff time.Duration, retries int) time.Duration {
	if currentBackoff >= maxBackoffDuration {
		return maxBackoffDuration
	}
	//nolint:gosec // it's a jitter.
	jitter := time.Duration(rand.Int63n(minBackoffDuration.Milliseconds())) * time.Millisecond
	newBackoff := (minBackoffDuration + jitter) * (1 << retries)
	return min(newBackoff, maxBackoffDuration)
}
