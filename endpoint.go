package httputil

import (
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
)

type (
	// Endpoint represents a registered HTTP endpoint.
	Endpoint struct {
		// Method is the HTTP method for this endpoint (e.g., "GET", "POST", "PUT", "DELETE").
		Method string
		// Path is the URL path for this endpoint (e.g., "/users", "/products/{id}").
		Path string
		// Handler is the [Handler] that will handle requests for this endpoint.
		Handler Handler

		guard Guard
	}

	// EndpointGroup represents a group of Endpoint definitions
	// allowing access to helper functions to define the group.
	EndpointGroup []Endpoint
)

func ProtectEndpoint(e Endpoint, g Guard) Endpoint {
	return Endpoint{
		Method:  e.Method,
		Path:    e.Path,
		Handler: e.Handler,
		guard:   g,
	}
}

// WithGuard adds the Guard as a GuardStack with the currently set Guard
// as the second Guard in the stack. It returns a new slice of EndpointGroup with
// the Guard set. The original endpoints are not modified.
func (eg EndpointGroup) WithGuard(guard Guard) EndpointGroup {
	if guard == nil {
		return eg
	}

	return cloneAndUpdate(eg, func(e *Endpoint) {
		if e.guard == nil {
			e.guard = guard
			return
		}

		e.guard = GuardStack{guard, e.guard}
	})
}

type handlerMiddlewareWrapper struct {
	handler    Handler
	middleware MiddlewareFunc
}

func (h handlerMiddlewareWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.middleware(h.handler).ServeHTTP(w, r)
}

func (h handlerMiddlewareWrapper) init(l *slog.Logger, v *validator.Validate, g Guard) {
	h.handler.init(l, v, g)
}

// WithMiddleware applies the given middleware to all provided
// endpoints. It returns a new slice of EndpointGroup with the middleware applied to
// their handlers. The original endpoints are not modified.
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

// WithPrefix prefixes the given path to all provided endpoints. It
// returns a new slice of EndpointGroup with the prefixed paths. The original
// endpoints are not modified.
func (eg EndpointGroup) WithPrefix(prefix string) EndpointGroup {
	return cloneAndUpdate(eg, func(e *Endpoint) {
		e.Path = prefix + e.Path
	})
}

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
