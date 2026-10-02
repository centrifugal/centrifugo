package proxy

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/stretchr/testify/require"
)

// blockingStream returns its responses, then blocks until its context is
// done, as a gRPC stream does, and returns the context error.
type blockingStream struct {
	ctx       context.Context
	responses []*proxyproto.StreamSubscribeResponse
	end       error
}

func (s *blockingStream) Recv() (*proxyproto.StreamSubscribeResponse, error) {
	if len(s.responses) > 0 {
		resp := s.responses[0]
		s.responses = s.responses[1:]
		return resp, nil
	}
	if s.end != nil {
		return nil, s.end
	}
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

// A stream closed by its own context, as for a subscription refused after
// the stream was opened, used to report the error to pubFunc, which blocked
// until the client disconnected waiting for a subscription which never
// became ready.
func TestReadStreamClosedByOwnerNotReported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := &blockingStream{ctx: ctx, responses: []*proxyproto.StreamSubscribeResponse{
		{Publication: &proxyproto.Publication{Data: []byte(`1`)}},
	}}
	var pubs int
	var errs []error
	done := make(chan struct{})
	go func() {
		defer close(done)
		readStream(ctx, cancel, stream, func(pub *proxyproto.Publication, err error) {
			if err != nil {
				errs = append(errs, err)
				return
			}
			pubs++
		})
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not return")
	}
	require.Equal(t, 1, pubs)
	require.Empty(t, errs)
}

// A stream ended by the other side is reported, so the subscription follows.
func TestReadStreamEndReported(t *testing.T) {
	for _, end := range []error{io.EOF, errors.New("stream failed")} {
		ctx, cancel := context.WithCancel(context.Background())
		var errs []error
		readStream(ctx, cancel, &blockingStream{ctx: ctx, end: end}, func(_ *proxyproto.Publication, err error) {
			errs = append(errs, err)
		})
		require.Equal(t, []error{end}, errs)
		require.Error(t, ctx.Err(), "the reader releases its context")
	}
}
