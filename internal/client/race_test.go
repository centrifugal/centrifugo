//go:build race

package client

// raceEnabled reports whether tests run with the race detector, which makes
// allocation counts unreliable: under it sync.Pool drops items at random.
const raceEnabled = true
