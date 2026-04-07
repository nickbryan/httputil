package httputil

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/nickbryan/httputil/problem"
)

// Ensure that our netHTTPHandler implements the Handler interface.
var _ http.Handler = &netHTTPHandler{} //nolint:exhaustruct // Compile time implementation check.

// netHTTPHandler allows a http.Handler to be used as a [Handler]. It will call
// a [Guard] and write errors as application/problem+text.
type netHTTPHandler struct {
	resolveOnce sync.Once
	handler     http.Handler
	logger      *slog.Logger
}

// WrapNetHTTPHandler wraps a standard http.Handler with additional
// functionality like optional guard and logging.
func WrapNetHTTPHandler(h http.Handler) http.Handler {
	return &netHTTPHandler{resolveOnce: sync.Once{}, handler: h, logger: nil}
}

// WrapNetHTTPHandlerFunc wraps an http.HandlerFunc in a netHTTPHandler to
// support additional features like guarding and logging.
func WrapNetHTTPHandlerFunc(h http.HandlerFunc) http.Handler {
	return &netHTTPHandler{resolveOnce: sync.Once{}, handler: h, logger: nil}
}

// ServeHTTP handles HTTP requests, applies the guard if present,
// and delegates to the wrapped handler. Errors are logged and returned as
// application/problem+text when the guard fails. It modifies the request
// if the guard provides a new instance.
func (h *netHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hc := handlerContextFrom(r.Context())
	if hc != nil {
		h.resolve(hc)
	}

	if h.logger == nil {
		panic(fmt.Sprintf("httputil: handler %T served without being registered on a Server (missing logger)", h))
	}

	var guard Guard
	if hc != nil {
		guard = hc.guard
	}

	if guard != nil {
		guardedRequest, ok := h.applyGuard(w, r, guard)
		if !ok {
			return
		}

		r = guardedRequest
	}

	h.handler.ServeHTTP(w, r)
}

// resolve sets logger from handlerContext. Fields already set are not
// overwritten.
func (h *netHTTPHandler) resolve(hc *handlerContext) {
	h.resolveOnce.Do(func() {
		if h.logger == nil {
			h.logger = hc.logger
		}
	})
}

// applyGuard runs the guard and writes an error response if it fails. Returns
// the (possibly modified) request and true on success, or false if the guard
// blocked the request and the response has already been written.
func (h *netHTTPHandler) applyGuard(w http.ResponseWriter, r *http.Request, guard Guard) (*http.Request, bool) {
	interceptedRequest, err := guard.Guard(r)
	if err != nil {
		h.writeGuardError(w, r, err)
		return nil, false
	}

	if interceptedRequest != nil {
		r = interceptedRequest
	}

	return r, true
}

// writeGuardError writes a problem response for a guard failure, logging
// unhandled errors.
func (h *netHTTPHandler) writeGuardError(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Content-Type", "application/problem+text")

	problemDetails, ok := errors.AsType[*problem.DetailedError](err)
	if !ok {
		problemDetails = problem.ServerError(r)
		err = fmt.Errorf("calling guard: %w", err)
		h.logger.ErrorContext(r.Context(), "Unhandled error received by net/http handler", slog.Any("error", err))
	}

	w.WriteHeader(problemDetails.Status)

	_, err = w.Write([]byte(problemDetails.Error())) //nolint:gosec // G705: writes structured problem error, not user input.
	if err != nil {
		err = fmt.Errorf("writing guard error: %w", err)
		h.logger.ErrorContext(r.Context(), "Failed to write error in net/http handler", slog.Any("error", err))
	}
}
