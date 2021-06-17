package clientcontext

import (
	"context"
	"encoding/json"
)

type ConnectionMeta struct {
	Meta json.RawMessage
}

type connectionMetaContextKey struct{}

func SetContextConnectionMeta(ctx context.Context, info ConnectionMeta) context.Context {
	ctx = context.WithValue(ctx, connectionMetaContextKey{}, info)
	return ctx
}
