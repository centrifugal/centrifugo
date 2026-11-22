// Package pgforward passes outbox batches of the Postgres brokers on to their
// fan-out brokers. It is apart from pgoutbox, which doesn't depend on
// centrifuge.
package pgforward

import "github.com/centrifugal/centrifuge"

// Publication is an outbox row to forward.
type Publication struct {
	Channel string
	Data    []byte
	Options centrifuge.PublishOptions
}

// Forward publishes an outbox batch to a fan-out broker, in order within a
// channel. An error doesn't stop the batch, it is passed to onError. The batch
// and the memory it references are only valid until Forward returns.
//
// It is a variable for Centrifugo PRO to replace.
var Forward = forwardEach

func forwardEach(broker centrifuge.Broker, pubs []Publication, onError func(error)) {
	for i := range pubs {
		if _, err := broker.Publish(pubs[i].Channel, pubs[i].Data, pubs[i].Options); err != nil {
			onError(err)
		}
	}
}
