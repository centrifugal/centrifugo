package proxy

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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

// firstMessageClient answers SubscribeUnidirectional with a stream whose
// first message comes right away, or, with afterCancel, only once the stream
// is cancelled: as a message already in flight comes back after a cancel.
type firstMessageClient struct {
	proxyproto.CentrifugoProxyClient
	afterCancel bool
}

func (c firstMessageClient) SubscribeUnidirectional(ctx context.Context, _ *proxyproto.SubscribeRequest, _ ...grpc.CallOption) (proxyproto.CentrifugoProxy_SubscribeUnidirectionalClient, error) {
	return &firstMessageStream{ctx: ctx, afterCancel: c.afterCancel}, nil
}

type firstMessageStream struct {
	grpc.ClientStream
	ctx         context.Context
	afterCancel bool
	sent        bool
}

func (s *firstMessageStream) Recv() (*proxyproto.StreamSubscribeResponse, error) {
	if !s.sent {
		s.sent = true
		if s.afterCancel {
			<-s.ctx.Done()
		}
		return &proxyproto.StreamSubscribeResponse{SubscribeResponse: &proxyproto.SubscribeResponse{}}, nil
	}
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

// A first message racing the timeout was accepted though the timeout had
// cancelled the stream: since a stream closed by its owner is not reported,
// the subscription stayed with a dead stream. Once the timeout fired the
// stream is refused.
func TestSubscribeStreamFirstMessageAfterTimeout(t *testing.T) {
	// Only the timeout cancels the stream here, so the first message comes
	// after it fired, whatever the timing.
	p := &SubscribeStreamProxy{
		config: Config{Timeout: configtypes.Duration(time.Millisecond)},
		client: firstMessageClient{afterCancel: true},
	}
	_, _, cancel, err := p.SubscribeStream(context.Background(), false, &proxyproto.SubscribeRequest{}, func(*proxyproto.Publication, error) {})
	require.ErrorContains(t, err, "timeout waiting for first message")
	require.Nil(t, cancel)

	p.config.Timeout = configtypes.Duration(time.Second)
	p.client = firstMessageClient{}
	resp, _, cancel, err := p.SubscribeStream(context.Background(), false, &proxyproto.SubscribeRequest{}, func(*proxyproto.Publication, error) {})
	require.NoError(t, err)
	require.NotNil(t, resp)
	cancel()
}
