package jwtverify

import (
	"sync"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/stretchr/testify/require"
)

// TestVerifierConcurrentReloadRace guards against a data race between token
// verification and a concurrent config reload (SIGHUP). VerifyConnectToken /
// VerifySubscribeToken used to read the verifier's signing/config fields unlocked
// while Reload rewrites them under mu.Lock - a data race that could also nil-deref
// algorithms/jwksManager when a reload switched auth mode. Verification now holds
// verifier.mu.RLock for its whole body. Run with -race.
func TestVerifierConcurrentReloadRace(t *testing.T) {
	cfgContainer, err := config.NewContainer(config.DefaultConfig())
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{HMACSecretKey: "secret"}, cfgContainer)
	require.NoError(t, err)

	done := make(chan struct{})
	var writerWg, readersWg sync.WaitGroup

	// Writer reloads the verifier config repeatedly, as a SIGHUP would.
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		configs := []VerifierConfig{
			{HMACSecretKey: "secret"},
			{HMACSecretKey: "secret2", HMACPreviousSecretKey: "secret"},
			{HMACSecretKey: "secret", Audience: "aud", Issuer: "iss"},
		}
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			_ = verifier.Reload(configs[i%len(configs)])
		}
	}()

	// Readers verify concurrently; results are irrelevant, only -race matters.
	for r := 0; r < 4; r++ {
		readersWg.Add(1)
		go func() {
			defer readersWg.Done()
			for i := 0; i < 3000; i++ {
				_, _ = verifier.VerifyConnectToken(jwtValid, false)
				_, _ = verifier.VerifySubscribeToken(subJWTValidCustomUserClaim, false)
			}
		}()
	}

	readersWg.Wait()
	close(done)
	writerWg.Wait()
}
