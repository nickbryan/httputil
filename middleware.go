package httputil

import (
	"context"
	"log/slog"
	"net/http"
)

// MiddlewareFunc defines a function type for HTTP middleware. A MiddlewareFunc
// takes a http.Handler as input and returns a new http.Handler that wraps the
// original action with additional logic (e.g., logging, authentication).
type MiddlewareFunc func(next http.Handler) http.Handler

// newPanicRecoveryMiddleware creates a MiddlewareFunc that recovers from panics
// within handlers. It logs the panic using the provided logger and returns a 500
// Internal Server Error to the client. It is important to note that any data
// written to the ResponseWriter before the panic will be sent to the client.
func newPanicRecoveryMiddleware(logger *slog.Logger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func(ctx context.Context) {
				if err := recover(); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					logger.ErrorContext(ctx, "Handler panicked", slog.Any("error", err))
				}
			}(r.Context())

			next.ServeHTTP(w, r)
		})
	}
}
