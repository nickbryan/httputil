package httputil

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil/problem"
)

// Ensure that our netHTTPHandler implements the Handler interface.
var _ Handler = &netHTTPHandler{} //nolint:exhaustruct // Compile time implementation check.

// netHTTPHandler allows a http.Handler to be used as a [Handler]. It will call
// a [Guard] and write errors as application/problem+text.
type netHTTPHandler struct {
	handler http.Handler
	guard   Guard
	logger  *slog.Logger
}

// NewNetHTTPHandler creates a new Handler that wraps the provided http.Handler
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandler(h http.Handler) Handler {
	return &netHTTPHandler{handler: h, guard: nil, logger: nil}
}

// NewNetHTTPHandlerFunc creates a new Handler that wraps the provided http.HandlerFunc
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandlerFunc(h http.HandlerFunc) Handler {
	return &netHTTPHandler{handler: h, guard: nil, logger: nil}
}

// ServeHTTP handles HTTP requests, applies the guard if present,
// and delegates to the wrapped handler. Errors are logged and returned as
// application/problem+text when the guard fails. It modifies the request
// if the guard provides a new instance.
func (h *netHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.guard != nil {
		interceptedRequest, err := h.guard.Guard(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/problem+text")

			var problemDetails *problem.DetailedError
			if !errors.As(err, &problemDetails) {
				problemDetails = problem.ServerError(r)
				err = fmt.Errorf("calling guard: %w", err)
				h.logger.ErrorContext(r.Context(), "net/http handler received an unhandled error", slog.Any("error", err))
			}

			w.WriteHeader(problemDetails.Status)

			_, err = w.Write([]byte(problemDetails.Error()))
			if err != nil {
				err = fmt.Errorf("writing guard error: %w", err)
				h.logger.ErrorContext(r.Context(), "net/http handler failed to write error", slog.Any("error", err))
			}

			return
		}

		if interceptedRequest != nil {
			r = interceptedRequest
		}
	}

	h.handler.ServeHTTP(w, r)
}

// with sets the logger and guard for the netHTTPHandler instance
// allowing dependencies to be injected by the server.
func (h *netHTTPHandler) with(l *slog.Logger, g Guard) Handler {
	return &netHTTPHandler{handler: h.handler, guard: g, logger: l}
}
