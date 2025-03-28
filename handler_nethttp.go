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
// a [RequestInterceptor] and write errors as application/problem+text.
type netHTTPHandler struct {
	handler            http.Handler
	requestInterceptor RequestInterceptor
	logger             *slog.Logger
}

// NewNetHTTPHandler creates a new Handler that wraps the provided http.Handler
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandler(h http.Handler) Handler {
	return &netHTTPHandler{handler: h, requestInterceptor: nil, logger: nil}
}

// NewNetHTTPHandlerFunc creates a new Handler that wraps the provided http.HandlerFunc
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandlerFunc(h http.HandlerFunc) Handler {
	return &netHTTPHandler{handler: h, requestInterceptor: nil, logger: nil}
}

// ServeHTTP handles HTTP requests, applies the request interceptor if present,
// and delegates to the wrapped handler. Errors are logged and returned as
// application/problem+text when the interceptor fails. It modifies the request
// if the interceptor provides a new instance.
func (h *netHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.requestInterceptor != nil {
		interceptedRequest, err := h.requestInterceptor.InterceptRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/problem+text")

			var problemDetails *problem.DetailedError
			if !errors.As(err, &problemDetails) {
				problemDetails = problem.ServerError(r)
				err = fmt.Errorf("calling request interceptor: %w", err)
				h.logger.ErrorContext(r.Context(), "net/http handler received an unhandled error", slog.Any("error", err))
			}

			w.WriteHeader(problemDetails.Status)

			_, err = w.Write([]byte(problemDetails.Error()))
			if err != nil {
				err = fmt.Errorf("writing request intercept error: %w", err)
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

// use sets the logger and request interceptor for the netHTTPHandler instance
// allowing dependencies to be injected by the server.
func (h *netHTTPHandler) use(l *slog.Logger, ri RequestInterceptor) {
	h.logger, h.requestInterceptor = l, ri
}
