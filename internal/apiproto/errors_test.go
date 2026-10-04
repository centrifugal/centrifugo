package apiproto

import (
	"go/ast"
	"go/parser"
	"go/token"
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

// TestErrorsDeclareTemporary checks every API error in errors.go sets
// Temporary explicitly - also to false - so that adding an error means
// deciding whether retrying it may help. It also checks apiErrors lists every
// error, so the tests above cover a new one.
func TestErrorsDeclareTemporary(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "errors.go", nil, 0)
	require.NoError(t, err)
	var declared []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Values) != 1 {
			return true
		}
		unary, ok := spec.Values[0].(*ast.UnaryExpr)
		if !ok {
			return true
		}
		lit, ok := unary.X.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if ident, ok := lit.Type.(*ast.Ident); !ok || ident.Name != "Error" {
			return true
		}
		name := spec.Names[0].Name
		declared = append(declared, name)
		hasTemporary := false
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Temporary" {
					hasTemporary = true
				}
			}
		}
		require.True(t, hasTemporary, "%s does not set Temporary: say whether retrying it may help", name)
		return true
	})
	require.Len(t, apiErrors, len(declared), "apiErrors must list every error of errors.go: %v", declared)
}
