package redisiam

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const (
	azureRedisScope = "https://redis.azure.com/.default"
	// Azure Redis disconnects a connection once its token expires, Microsoft recommends
	// sending AUTH with a new token at least 3 minutes before expiry.
	azureReauthBeforeExpiry = 3 * time.Minute
	// Re-authentication is scheduled at a random 60-80% of the remaining token lifetime
	// (staggers AUTH commands). MSAL returns a cached token until it's within 5 minutes
	// of expiry, so several re-authentications may send the same token – then the last
	// one before the 3 minute deadline gets a new token. This leaves room for retries
	// when Microsoft Entra is temporarily unavailable.
	azureReauthRatioMin = 0.6
	azureReauthRatioMax = 0.8
	// azureRetryDelay is the delay before the next attempt to get a new token when
	// getting it failed but the previous token is still valid.
	azureRetryDelay = 30 * time.Second
	// azureMinReauthDelay protects from a tight re-authentication loop.
	azureMinReauthDelay = 10 * time.Second
)

func newAzureProvider(user string, timeout time.Duration) (Provider, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("create Azure default credential for Redis Entra auth: %w", err)
	}
	return newAzureTokenCredentialProvider(user, cred, timeout), nil
}

func newAzureTokenCredentialProvider(user string, cred azcore.TokenCredential, timeout time.Duration) Provider {
	p := &azureTokenProvider{user: user, cred: cred, timeout: timeout}
	return p.credentials
}

type azureTokenProvider struct {
	user    string
	cred    azcore.TokenCredential
	timeout time.Duration

	mu        sync.Mutex
	lastToken azcore.AccessToken
}

func (p *azureTokenProvider) credentials() (Credentials, error) {
	token, refreshAfter, err := p.token(time.Now())
	if err != nil {
		return Credentials{}, err
	}
	username := p.user
	if username == "" {
		username, err = azureTokenObjectID(token.Token)
		if err != nil {
			return Credentials{}, fmt.Errorf("extract object ID from Microsoft Entra access token (set user explicitly): %w", err)
		}
	}
	return Credentials{
		Username:     username,
		Password:     token.Token,
		RefreshAfter: refreshAfter,
	}, nil
}

func (p *azureTokenProvider) token(now time.Time) (azcore.AccessToken, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	token, err := p.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{azureRedisScope}})
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		// Keep using the previous token while it's valid, retrying to get a new one. Otherwise,
		// the connection is closed and can't be re-established until Microsoft Entra recovers.
		last := p.lastToken
		if last.Token == "" || last.ExpiresOn.Sub(now) <= azureReauthBeforeExpiry {
			return azcore.AccessToken{}, time.Time{}, fmt.Errorf("get Microsoft Entra access token: %w", err)
		}
		return last, clampReauthTime(now.Add(azureRetryDelay), last.ExpiresOn, now), nil
	}
	if token.ExpiresOn.After(p.lastToken.ExpiresOn) {
		p.lastToken = token
	}
	return token, azureReauthTime(token.ExpiresOn, now), nil
}

func azureReauthTime(expiresOn time.Time, now time.Time) time.Time {
	ratio := azureReauthRatioMin + randFloat64()*(azureReauthRatioMax-azureReauthRatioMin)
	reauthAt := now.Add(time.Duration(float64(expiresOn.Sub(now)) * ratio))
	return clampReauthTime(reauthAt, expiresOn, now)
}

// randFloat64 returns a random float64 in [0, 1).
func randFloat64() float64 {
	var b [8]byte
	_, _ = rand.Read(b[:]) // Never returns an error.
	return float64(binary.LittleEndian.Uint64(b[:])>>11) / (1 << 53)
}

// clampReauthTime makes sure re-authentication happens before the expiry deadline,
// but not in a tight loop.
func clampReauthTime(reauthAt time.Time, expiresOn time.Time, now time.Time) time.Time {
	if deadline := expiresOn.Add(-azureReauthBeforeExpiry); reauthAt.After(deadline) {
		reauthAt = deadline
	}
	if minReauthAt := now.Add(azureMinReauthDelay); reauthAt.Before(minReauthAt) {
		reauthAt = minReauthAt
	}
	return reauthAt
}

// azureTokenObjectID extracts the oid claim (object ID of the service principal or
// managed identity) which Azure Redis expects as a username. The token is obtained
// from Microsoft Entra directly, so its signature is not verified here.
func azureTokenObjectID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("malformed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims struct {
		OID string `json:"oid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("unmarshal JWT payload: %w", err)
	}
	if claims.OID == "" {
		return "", errors.New("no oid claim in token")
	}
	return claims.OID, nil
}
