package apiproto

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// apiErrors lists every error Centrifugo returns in API replies.
var apiErrors = []*Error{
	ErrorInternal,
	ErrorUnknownChannel,
	ErrorNotFound,
	ErrorBadRequest,
	ErrorNotAvailable,
	ErrorTooManyRequests,
	ErrorUnrecoverablePosition,
	ErrorConflict,
}

// TestErrorsTemporary checks which errors say a retry may succeed: the ones
// caused by a temporary condition, not by the request.
func TestErrorsTemporary(t *testing.T) {
	temporary := map[uint32]bool{
		ErrorInternal.Code:        true,
		ErrorTooManyRequests.Code: true,
	}
	for _, e := range apiErrors {
		require.Equal(t, temporary[e.Code], e.Temporary, "error %d", e.Code)
	}
}

// TestErrorsMappedExplicitly checks every API error has its own HTTP and gRPC
// status: an unmapped one falls back to 500 and Internal, which is right for
// the internal error only.
func TestErrorsMappedExplicitly(t *testing.T) {
	for _, e := range apiErrors {
		if e == ErrorInternal {
			continue
		}
		require.NotEqual(t, http.StatusInternalServerError, MapErrorToHTTPCode(e), "error %d", e.Code)
		require.NotEqual(t, codes.Internal, MapErrorToGRPCCode(e), "error %d", e.Code)
	}
	require.Equal(t, http.StatusTooManyRequests, MapErrorToHTTPCode(ErrorTooManyRequests))
	require.Equal(t, codes.ResourceExhausted, MapErrorToGRPCCode(ErrorTooManyRequests))
}

// TestErrorsTemporaryJSON checks what clients receive: temporary is sent
// for a temporary error and left out otherwise.
func TestErrorsTemporaryJSON(t *testing.T) {
	data, err := EncodeError(ErrorInternal)
	require.NoError(t, err)
	require.JSONEq(t, `{"code":100,"message":"internal server error","temporary":true}`, string(data))

	data, err = EncodeError(ErrorTooManyRequests)
	require.NoError(t, err)
	require.JSONEq(t, `{"code":111,"message":"too many requests","temporary":true}`, string(data))

	data, err = EncodeError(ErrorBadRequest)
	require.NoError(t, err)
	require.JSONEq(t, `{"code":107,"message":"bad request"}`, string(data))
}
