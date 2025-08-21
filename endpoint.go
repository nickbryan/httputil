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
		// Handler is the [http.Handler] that will handle requests for this endpoint.
		Handler http.Handler

		guard Guard
	}

	// EndpointGroup represents a group of Endpoint definitions allowing access to
	// helper functions to define the group.
	EndpointGroup []Endpoint

	// GuardStack represents multiple Guard instances that
	// will be run in order.
	GuardStack []Guard
)

// Ensure that GuardStack implements the Guard
// interface.
var _ Guard = GuardStack{}

// Guard will run each Guard in order starting from 0.
// It will continue iteration until a non nil http.Request or error is returned,
// it will then return the http.Request and error of that call.
func (gs GuardStack) Guard(r *http.Request) (*http.Request, error) {
	for _, g := range gs {
		interceptedRequest, err := g.Guard(r)
		if err != nil {
			return nil, err //nolint:wrapcheck // Allow the Guard to determine result.
		}

		r = interceptedRequest
	}

	return r, nil
}

// NewEndpointWithGuard associates the given Guard
// with the specified Endpoint. It returns a new Endpoint with the
// Guard applied. The original Endpoint remains unmodified.
func NewEndpointWithGuard(e Endpoint, g Guard) Endpoint {
	return Endpoint{
		Method:  e.Method,
		Path:    e.Path,
		Handler: e.Handler,
		guard:   g,
	}
}

// WithGuard adds the Guard as a
// GuardStack with the currently set Guard as the
// second Guard in the stack. It returns a new slice of
// EndpointGroup with the Guard set. The original endpoints are not
// modified.
func (eg EndpointGroup) WithGuard(g Guard) EndpointGroup {
	if g == nil {
		return eg
	}

	return cloneAndUpdate(eg, func(e *Endpoint) {
		if e.guard == nil {
			e.guard = g
			return
		}

		e.guard = GuardStack{g, e.guard}
	})
}

// handlerMiddlewareWrapper is a struct that wraps a Handler with MiddlewareFunc
// to ensure that dependencies are passed through the middleware to the Handler.
type handlerMiddlewareWrapper struct {
	handler    http.Handler
	middleware MiddlewareFunc
}

func (h handlerMiddlewareWrapper) setCodec(c ServerCodec) {
	if codecSetter, ok := h.handler.(interface{ setCodec(c ServerCodec) }); ok {
		codecSetter.setCodec(c)
	}
}

func (h handlerMiddlewareWrapper) setGuard(g Guard) {
	if guardSetter, ok := h.handler.(interface{ setGuard(g Guard) }); ok {
		guardSetter.setGuard(g)
	}
}

func (h handlerMiddlewareWrapper) setLogger(l *slog.Logger) {
	if logSetter, ok := h.handler.(interface{ setLogger(l *slog.Logger) }); ok {
		logSetter.setLogger(l)
	}
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
			Method:  endpoint.Method,
			Path:    endpoint.Path,
			Handler: endpoint.Handler,
			guard:   endpoint.guard,
		}

		update(&e)

		es = append(es, e)
	}

	return es
}
