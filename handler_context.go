package httputil

import (
	"context"
	"log/slog"
)

// handlerContext holds handler dependencies injected by Server.Register into
// request context, following the same pattern as net/http's ServerContextKey.
// Both the writer (Server.Register) and readers (handler.resolve,
// netHTTPHandler.resolve) are unexported internals in this package.
type handlerContext struct {
	codec  ServerCodec
	guard  Guard
	logger *slog.Logger
}

// handlerCtxKey is the context key for handlerContext values.
type handlerCtxKey struct{}

// handlerContextFrom extracts the handlerContext from ctx, returning nil if absent.
func handlerContextFrom(ctx context.Context) *handlerContext {
	hc, _ := ctx.Value(handlerCtxKey{}).(*handlerContext)
	return hc
}
