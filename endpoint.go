package httputil

import (
	"net/http"
	"slices"
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

// WithMiddleware applies the given middlewares to all provided endpoints. It
// returns a new EndpointGroup with the middlewares applied to their handlers.
// The original endpoints are not modified. Nil middlewares are skipped.
//
// Ordering:
//   - Within a single call, middlewares run in the order they are given:
//     eg.WithMiddleware(a, b, c) runs a first on each request, then b, then c,
//     then the handler.
//   - Across calls, each call wraps the existing handler, so the most recent
//     call is outermost: eg.WithMiddleware(a).WithMiddleware(b) runs b first,
//     then a, then the handler.
//
// The across-call ordering supports hierarchical composition: when an outer
// EndpointGroup is built from inner ones (for example, by spreading inner
// endpoints into a new group and calling WithMiddleware on the result), the
// outer call's middleware wraps everything from the inner calls. This differs
// from [WithClientInterceptor], whose across-call ordering is FIFO because
// client interceptors form a single flat chain rather than a nested
// composition.
func (eg EndpointGroup) WithMiddleware(middlewares ...MiddlewareFunc) EndpointGroup {
	return cloneAndUpdate(eg, func(e *Endpoint) {
		for _, m := range slices.Backward(middlewares) {
			if m != nil {
				e.Handler = m(e.Handler)
			}
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
