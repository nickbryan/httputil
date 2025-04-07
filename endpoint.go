package httputil

import (
	"log/slog"
	"net/http"
)

type (

	// Endpoint represents an HTTP endpoint with a method, path, and handler.
	Endpoint struct {
		// Method is the HTTP method for this endpoint (e.g., "GET", "POST", "PUT",
		// "DELETE").
		Method string
		// Path is the URL path for this endpoint (e.g., "/users", "/products/{id}").
		Path string
		// Handler is the [Handler] that will handle requests for this endpoint.
		Handler Handler

		requestInterceptor RequestInterceptor
	}

	// EndpointGroup represents a group of Endpoint definitions allowing access to
	// helper functions to define the group.
	EndpointGroup []Endpoint

	// RequestInterceptorStack represents multiple RequestInterceptor instances that
	// will be run in order.
	RequestInterceptorStack []RequestInterceptor
)

// Ensure that RequestInterceptorStack implements the RequestInterceptor
// interface.
var _ RequestInterceptor = RequestInterceptorStack{}

// InterceptRequest will run each RequestInterceptor in order starting from 0.
// It will continue iteration until a non nil http.Request or error is returned,
// it will then return the http.Request and error of that call.
func (ris RequestInterceptorStack) InterceptRequest(r *http.Request) (*http.Request, error) {
	for _, ri := range ris {
		interceptedRequest, err := ri.InterceptRequest(r)
		if err != nil {
			return nil, err //nolint:wrapcheck // Allow the RequestInterceptor to determine result.
		}

		r = interceptedRequest
	}

	return r, nil
}

// NewEndpointWithRequestInterceptor associates the given RequestInterceptor
// with the specified Endpoint. It returns a new Endpoint with the
// RequestInterceptor applied. The original Endpoint remains unmodified.
func NewEndpointWithRequestInterceptor(e Endpoint, ri RequestInterceptor) Endpoint {
	return Endpoint{
		Method:             e.Method,
		Path:               e.Path,
		Handler:            e.Handler,
		requestInterceptor: ri,
	}
}

// WithRequestInterceptor adds the RequestInterceptor as a
// RequestInterceptorStack with the currently set RequestInterceptor as the
// second RequestInterceptor in the stack. It returns a new slice of
// EndpointGroup with the RequestInterceptor set. The original endpoints are not
// modified.
func (eg EndpointGroup) WithRequestInterceptor(ri RequestInterceptor) EndpointGroup {
	if ri == nil {
		return eg
	}

	return cloneAndUpdate(eg, func(e *Endpoint) {
		if e.requestInterceptor == nil {
			e.requestInterceptor = ri
			return
		}

		e.requestInterceptor = RequestInterceptorStack{ri, e.requestInterceptor}
	})
}

// handlerMiddlewareWrapper is a struct that wraps a Handler with MiddlewareFunc
// to ensure that dependencies are passed through the middleware to the Handler.
type handlerMiddlewareWrapper struct {
	handler    Handler
	middleware MiddlewareFunc
}

// with initializes the underlying handler with a logger and a request interceptor to ensure that
// the dependencies are passed through the middleware.
func (h handlerMiddlewareWrapper) with(l *slog.Logger, ri RequestInterceptor) Handler {
	return h.handler.with(l, ri)
}

// ServeHTTP processes HTTP requests using the wrapped handler and middleware,
// allowing additional middleware logic.
func (h handlerMiddlewareWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.middleware(h.handler).ServeHTTP(w, r)
}

// WithMiddleware applies the given middleware to all provided endpoints. It
// returns a new slice of EndpointGroup with the middleware applied to their
// handlers. The original endpoints are not modified.
func (eg EndpointGroup) WithMiddleware(middleware MiddlewareFunc) EndpointGroup {
	if middleware == nil {
		return eg
	}

	return cloneAndUpdate(eg, func(e *Endpoint) {
		e.Handler = handlerMiddlewareWrapper{
			handler:    e.Handler,
			middleware: middleware,
		}
	})
}

// WithPrefix prefixes the given path to all provided endpoints. It returns a
// new slice of EndpointGroup with the prefixed paths. The original endpoints
// are not modified.
func (eg EndpointGroup) WithPrefix(prefix string) EndpointGroup {
	return cloneAndUpdate(eg, func(e *Endpoint) {
		e.Path = prefix + e.Path
	})
}

// cloneAndUpdate creates a copy of the provided endpoints, applies the update
// function to each copy, and returns the new list.
func cloneAndUpdate(endpoints []Endpoint, update func(e *Endpoint)) []Endpoint {
	es := make([]Endpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		e := Endpoint{
			Method:             endpoint.Method,
			Path:               endpoint.Path,
			Handler:            endpoint.Handler,
			requestInterceptor: endpoint.requestInterceptor,
		}

		update(&e)

		es = append(es, e)
	}

	return es
}
