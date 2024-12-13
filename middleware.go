package httputil

import (
	"log/slog"
	"net/http"

	"golang.org/x/net/context"
)

// MiddlewareFunc defines a function type for HTTP middleware.
// A MiddlewareFunc takes an http.Handler as input and returns a new http.Handler
// that wraps the original handler with additional logic (e.g., logging, authentication).
type MiddlewareFunc func(next http.Handler) http.Handler

// NewPanicRecoveryMiddleware creates a MiddlewareFunc that recovers from panics within handlers.
// It logs the panic using the provided logger and returns a 500 Internal Server Error to the client.
// It is important to note that any data written to the ResponseWriter before the panic will be sent to the client.
func NewPanicRecoveryMiddleware(logger *slog.Logger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func(ctx context.Context) {
				if err := recover(); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					// TODO: check the other version of this, handle string and error at least.
					logger.ErrorContext(ctx, "Handler has panicked", slog.Any("error", err))
				}
			}(r.Context())

			next.ServeHTTP(w, r)
		})
	}
}
