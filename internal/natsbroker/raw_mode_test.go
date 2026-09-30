package natsbroker

import (
	"errors"
	"testing"

	"github.com/centrifugal/centrifuge"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type testEventHandler struct {
	publications []*centrifuge.Publication
}

func (h *testEventHandler) HandlePublication(_ string, pub *centrifuge.Publication, _ centrifuge.StreamPosition, _ bool, _ *centrifuge.Publication) error {
	h.publications = append(h.publications, pub)
	return nil
}

func (h *testEventHandler) HandleJoin(string, *centrifuge.ClientInfo) error { return nil }

func (h *testEventHandler) HandleLeave(string, *centrifuge.ClientInfo) error { return nil }

// A message which comes from Nats in raw mode is delivered unless the data
// check says no.
func TestRawModeDataCheck(t *testing.T) {
	handler := &testEventHandler{}
	sub := &nats.Subscription{Subject: "raw.news"}
	b := &NatsBroker{
		config:       Config{},
		subs:         map[channelID]subWrapper{"raw.news": {sub: sub, origChannel: "news"}},
		eventHandler: handler,
	}
	b.config.RawMode.Enabled = true
	b.config.RawMode.Prefix = "raw."

	// Without a check everything is delivered.
	b.handleClientMessage("raw.news", []byte(`not json`), sub)
	require.Len(t, handler.publications, 1)

	var checked []string
	b.SetRawModeDataCheck(func(channel string, data []byte) error {
		checked = append(checked, channel)
		if string(data) == `not json` {
			return errors.New("data is not valid JSON")
		}
		return nil
	})
	b.handleClientMessage("raw.news", []byte(`not json`), sub)
	require.Len(t, handler.publications, 1)
	b.handleClientMessage("raw.news", []byte(`{"a":1}`), sub)
	require.Len(t, handler.publications, 2)
	require.Equal(t, `{"a":1}`, string(handler.publications[1].Data))
	require.Equal(t, "news", handler.publications[1].Channel)
	// The check is given the channel of Centrifugo, not the subject of Nats.
	require.Equal(t, []string{"news", "news"}, checked)
}
