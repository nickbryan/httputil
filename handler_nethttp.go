package httputil

import (
	"log/slog"
	"net/http"
)

// Ensure that our netHTTPHandler implements the Handler interface.
var _ Handler = netHTTPHandler{} //nolint:exhaustruct // Compile time implementation check.

type netHTTPHandler struct {
	handler http.Handler
}

// NewNetHTTPHandler creates a new Handler that wraps the provided http.Handler
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandler(h http.Handler) Handler {
	return netHTTPHandler{handler: h}
}

// NewNetHTTPHandlerFunc creates a new Handler that wraps the provided http.HandlerFunc
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandlerFunc(h http.HandlerFunc) Handler {
	return netHTTPHandler{handler: h}
}

func (h netHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

func (h netHTTPHandler) use(_ *slog.Logger, _ RequestInterceptor) {}
