package httputil

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// MiddlewareFunc defines a function type for HTTP middleware. A MiddlewareFunc
// takes a http.Handler as input and returns a new http.Handler that wraps the
// original action with additional logic.
type MiddlewareFunc func(next http.Handler) http.Handler

// newPanicRecoveryMiddleware creates a MiddlewareFunc that recovers from panics
// within handlers. It logs the panic using the provided logger and returns a 500
// Internal Server Error to the doer. It is important to note that any data
// written to the ResponseWriter before the panic will be sent to the doer.
func newPanicRecoveryMiddleware(logger *slog.Logger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func(ctx context.Context) {
				if err := recover(); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					logger.ErrorContext(
						ctx,
						"Handler panicked",
						slog.Any("error", err),
						slog.String("stack", string(debug.Stack())),
					)
				}
			}(r.Context())

			next.ServeHTTP(w, r)
		})
	}
}

// newMaxBodySizeMiddleware creates a middleware that enforces an upper limit on
// the size of request bodies. This is important to:
//   - Protect the server from being overwhelmed by excessively large requests.
//   - Prevent potential abuse, such as denial-of-service (DoS) attacks, where
//     malicious clients send extremely large payloads to consume server resources.
//   - Ensure efficient use of server memory and processing resources.
//
// If the ContentLength exceeds maxBytes, it responds with a 413 status code. It
// also wraps the request body with http.MaxBytesReader to enforce the limit
// during reading.
func newMaxBodySizeMiddleware(logger *slog.Logger, maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				http.Error(w, "Request entity too large", http.StatusRequestEntityTooLarge)
				logger.WarnContext(
					r.Context(),
					"Request body exceeds max bytes limit",
					slog.Int64("max_bytes", maxBytes),
				)

				return
			}

			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}

			next.ServeHTTP(w, r)
		})
	}
}
