package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func testHandler() http.Handler {
	fn := func(rw http.ResponseWriter, req *http.Request) {}
	return http.HandlerFunc(fn)
}

func TestAPIKeyAuthEmptyKey(t *testing.T) {
	ts := httptest.NewServer(NewAPIKeyAuth("").Middleware(testHandler()))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestAPIKeyAuthMissingAuthKey(t *testing.T) {
	ts := httptest.NewServer(NewAPIKeyAuth("test").Middleware(testHandler()))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestAPIKeyAuthXAPIKey(t *testing.T) {
	ts := httptest.NewServer(NewAPIKeyAuth("test").Middleware(testHandler()))
	defer ts.Close()

	req, err := http.NewRequest("POST", ts.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", "test")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusOK)

	req, err = http.NewRequest("POST", ts.URL, nil)
	require.NoError(t, err)
	req.Header.Set("x-api-key", "test")
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusOK)
}

func TestAPIKeyAuthAuthorizationHeader(t *testing.T) {
	ts := httptest.NewServer(NewAPIKeyAuth("test").Middleware(testHandler()))
	defer ts.Close()

	req, err := http.NewRequest("POST", ts.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "bearer test")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusUnauthorized)

	req, err = http.NewRequest("POST", ts.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "test")
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusUnauthorized)

	req, err = http.NewRequest("POST", ts.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "apikey test test")
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusUnauthorized)

	req, err = http.NewRequest("POST", ts.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "apikey test")
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusOK)
}

func TestAPIKeyAuthQueryParam(t *testing.T) {
	ts := httptest.NewServer(NewAPIKeyAuth("test").Middleware(testHandler()))
	defer ts.Close()

	res, err := http.Post(ts.URL+"?api_key=t", "application/json", nil)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusUnauthorized)

	res, err = http.Post(ts.URL+"?api_key=test", "application/json", nil)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusOK)
}
