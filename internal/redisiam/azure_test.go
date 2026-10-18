package redisiam

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/require"
)

type testTokenCredential struct {
	token     azcore.AccessToken
	err       error
	gotScopes []string
}

func (c *testTokenCredential) GetToken(_ context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.gotScopes = opts.Scopes
	return c.token, c.err
}

func testJWT(payload string) string {
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." +
		enc.EncodeToString([]byte(payload)) + ".signature"
}

func TestAzureTokenCredentialProvider(t *testing.T) {
	expiresOn := time.Now().Add(time.Hour)
	cred := &testTokenCredential{token: azcore.AccessToken{
		Token:     testJWT(`{"oid":"11111111-2222-3333-4444-555555555555","aud":"https://redis.azure.com"}`),
		ExpiresOn: expiresOn,
	}}

	creds, err := newAzureTokenCredentialProvider("", cred, time.Second)()
	require.NoError(t, err)
	require.Equal(t, []string{azureRedisScope}, cred.gotScopes)
	require.Equal(t, "11111111-2222-3333-4444-555555555555", creds.Username)
	require.Equal(t, cred.token.Token, creds.Password)
	// 60-80% of the remaining hour.
	require.True(t, creds.RefreshAfter.After(time.Now().Add(35*time.Minute)))
	require.True(t, creds.RefreshAfter.Before(expiresOn.Add(-11*time.Minute)))

	// Explicit user takes precedence over the oid claim.
	creds, err = newAzureTokenCredentialProvider("explicit-user", cred, time.Second)()
	require.NoError(t, err)
	require.Equal(t, "explicit-user", creds.Username)
}

func TestAzureTokenCredentialProviderErrors(t *testing.T) {
	_, err := newAzureTokenCredentialProvider("", &testTokenCredential{err: errors.New("boom")}, time.Second)()
	require.ErrorContains(t, err, "boom")

	noOID := &testTokenCredential{token: azcore.AccessToken{Token: testJWT(`{"aud":"x"}`), ExpiresOn: time.Now().Add(time.Hour)}}
	_, err = newAzureTokenCredentialProvider("", noOID, time.Second)()
	require.ErrorContains(t, err, "no oid claim")

	// Token is not parsed when user is set explicitly.
	opaque := &testTokenCredential{token: azcore.AccessToken{Token: "opaque", ExpiresOn: time.Now().Add(time.Hour)}}
	_, err = newAzureTokenCredentialProvider("user", opaque, time.Second)()
	require.NoError(t, err)
	_, err = newAzureTokenCredentialProvider("", opaque, time.Second)()
	require.ErrorContains(t, err, "malformed JWT")
}

func TestAzureTokenCredentialProviderFallback(t *testing.T) {
	now := time.Now()
	cred := &testTokenCredential{token: azcore.AccessToken{Token: "token-1", ExpiresOn: now.Add(time.Hour)}}
	p := &azureTokenProvider{user: "user", cred: cred, timeout: time.Second}

	token, _, err := p.token(now)
	require.NoError(t, err)
	require.Equal(t, "token-1", token.Token)

	// Microsoft Entra unavailable – previous token is reused, new token is retried soon.
	cred.token, cred.err = azcore.AccessToken{}, errors.New("entra unavailable")
	token, refreshAfter, err := p.token(now.Add(10 * time.Minute))
	require.NoError(t, err)
	require.Equal(t, "token-1", token.Token)
	require.Equal(t, now.Add(10*time.Minute+azureRetryDelay), refreshAfter)

	// Close to the deadline – retry no later than the deadline.
	_, refreshAfter, err = p.token(now.Add(56*time.Minute + 45*time.Second))
	require.NoError(t, err)
	require.Equal(t, now.Add(time.Hour-azureReauthBeforeExpiry), refreshAfter)

	// Previous token is too close to expiry to be used.
	_, _, err = p.token(now.Add(57 * time.Minute))
	require.ErrorContains(t, err, "entra unavailable")

	// Recovered.
	cred.token, cred.err = azcore.AccessToken{Token: "token-2", ExpiresOn: now.Add(2 * time.Hour)}, nil
	token, _, err = p.token(now.Add(57 * time.Minute))
	require.NoError(t, err)
	require.Equal(t, "token-2", token.Token)
}

func TestAzureTokenCredentialProviderNoFallbackWithoutToken(t *testing.T) {
	p := &azureTokenProvider{user: "user", cred: &testTokenCredential{err: errors.New("boom")}, timeout: time.Second}
	_, _, err := p.token(time.Now())
	require.ErrorContains(t, err, "boom")
}

func TestAzureReauthTime(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		reauthAt := azureReauthTime(now.Add(time.Hour), now)
		require.False(t, reauthAt.Before(now.Add(36*time.Minute)))
		require.False(t, reauthAt.After(now.Add(48*time.Minute)))
	}
	// Never later than the deadline before expiry.
	require.Equal(t, now.Add(2*time.Minute), azureReauthTime(now.Add(5*time.Minute), now))
	// Token close to expiry – do not re-authenticate in a tight loop.
	require.Equal(t, now.Add(azureMinReauthDelay), azureReauthTime(now.Add(time.Minute), now))
}

// Repeated re-authentication with the same (cached) token converges to the deadline
// with a bounded number of AUTH commands.
func TestAzureReauthTimeConverges(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	expiresOn := now.Add(time.Hour)
	deadline := expiresOn.Add(-azureReauthBeforeExpiry)
	steps := 0
	for now.Before(deadline) {
		now = azureReauthTime(expiresOn, now)
		steps++
		require.Less(t, steps, 10)
	}
	// The loop guard may shift the last step up to azureMinReauthDelay past the deadline.
	require.False(t, now.After(deadline.Add(azureMinReauthDelay)))
}
