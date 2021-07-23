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

func GetContextConnectionMeta(ctx context.Context) (ConnectionMeta, bool) {
	if val := ctx.Value(connectionMetaContextKey{}); val != nil {
		values, ok := val.(ConnectionMeta)
		return values, ok
	}
	return ConnectionMeta{}, false
}
