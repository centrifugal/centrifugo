package proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type httpMapPublishHandleTestCase struct {
	*tools.CommonHTTPProxyTestCase
	mapPublishProxyHandler *MapPublishHandler
	channelOpts            configtypes.ChannelOptions
}

func newMapPublishHandleHTTPTestCase(ctx context.Context, endpoint string, opts configtypes.ChannelOptions) httpMapPublishHandleTestCase {
	commonProxyTestCase := tools.NewCommonHTTPProxyTestCase(ctx)

	mapPublishProxy, err := NewHTTPMapPublishProxy(getTestHttpProxy(commonProxyTestCase, endpoint))
	if err != nil {
		log.Fatalln("could not create http map publish proxy: ", err)
	}

	mapPublishProxyHandler := NewMapPublishHandler(MapPublishHandlerConfig{
		Proxies: map[string]MapPublishProxy{
			"test": mapPublishProxy,
		},
	})

	return httpMapPublishHandleTestCase{commonProxyTestCase, mapPublishProxyHandler, opts}
}

func (c httpMapPublishHandleTestCase) invokeHandle(event centrifuge.MapPublishEvent) (centrifuge.MapPublishReply, error) {
	handler := c.mapPublishProxyHandler.Handle(c.Node)
	return handler(c.Client, event, c.channelOpts, PerCallData{})
}

// TestHandleMapPublishProxyResultDataValidated ensures data returned by a map
// publish proxy is validated against the channel publication_data_format — the
// proxy can replace what the client sent, so checking only the client data in
// the client handler is not enough.
func TestHandleMapPublishProxyResultDataValidated(t *testing.T) {
	// Not a JSON object, while the channel requires one.
	badDataB64 := base64.StdEncoding.EncodeToString([]byte(`"just a string"`))
	chOpts := configtypes.ChannelOptions{
		SubscriptionType:      "map",
		PublicationDataFormat: configtypes.PublicationDataFormatJSONObject,
		Map: configtypes.MapConfig{
			Mode:                "persistent",
			PublishProxyEnabled: true,
			PublishProxyName:    "test",
		},
	}

	testCase := newMapPublishHandleHTTPTestCase(context.Background(), "/map_publish", chOpts)
	testCase.Mux.HandleFunc("/map_publish", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"result": {"key": "k", "b64data": "%s"}}`, badDataB64)))
	})
	defer testCase.Teardown()

	_, err := testCase.invokeHandle(centrifuge.MapPublishEvent{
		Channel: "test_channel",
		Key:     "k",
		Data:    []byte(`{"valid":"object"}`), // client data is fine, proxy data is not
	})
	require.Equal(t, centrifuge.ErrorBadRequest, err)
}
