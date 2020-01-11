package health

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func TestHealthHandler(t *testing.T) {
	node := nodeWithMemoryEngine()
	h := NewHandler(node, Config{})

	ts := httptest.NewServer(h)
	defer ts.Close()

	res, err := http.Get(ts.URL)
	require.NoError(t, err)
	require.Equal(t, res.StatusCode, http.StatusOK)
	defer res.Body.Close()

	require.Equal(t, "application/json", res.Header.Get("Content-Type"))

	data, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, []byte(`{}`), data)
}

func nodeWithMemoryEngine() *centrifuge.Node {
	c := centrifuge.DefaultConfig
	n, err := centrifuge.New(c)
	if err != nil {
		panic(err)
	}
	err = n.Run()
	if err != nil {
		panic(err)
	}
	return n
}
