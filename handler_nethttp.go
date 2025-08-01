package httputil

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil/problem"
)

// Ensure that our netHTTPHandler implements the Handler interface.
var _ http.Handler = &netHTTPHandler{} //nolint:exhaustruct // Compile time implementation check.

// netHTTPHandler allows a http.Handler to be used as a [Handler]. It will call
// a [Guard] and write errors as application/problem+text.
type netHTTPHandler struct {
	guard   Guard
	handler http.Handler
	logger  *slog.Logger
}

// WrapNetHTTPHandler wraps a standard http.Handler with additional
// functionality like optional guard and logging.
func WrapNetHTTPHandler(h http.Handler) http.Handler {
	return &netHTTPHandler{handler: h, guard: nil, logger: nil}
}

// WrapNetHTTPHandlerFunc wraps an http.HandlerFunc in a netHTTPHandler to
// support additional features like guarding and logging.
func WrapNetHTTPHandlerFunc(h http.HandlerFunc) http.Handler {
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

// setGuard sets the guard for the handler if it has not already been set.
// This method is called by the Server when registering endpoints with guards.
func (h *netHTTPHandler) setGuard(g Guard) {
	if h.guard == nil {
		h.guard = g
	}
}

// setLogger sets the logger for the handler if it has not already been set.
// This method is called by the Server when registering endpoints to provide
// consistent logging across all handlers.
func (h *netHTTPHandler) setLogger(l *slog.Logger) {
	if h.logger == nil {
		h.logger = l
	}
}
