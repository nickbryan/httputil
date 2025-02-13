package httputil

import (
	"net/http"
)

// Endpoint represents a registered HTTP endpoint.
type Endpoint struct {
	// Method is the HTTP method for this endpoint (e.g., "GET", "POST", "PUT", "DELETE").
	Method string
	// Path is the URL path for this endpoint (e.g., "/users", "/products/{id}").
	Path string
	// Handler is the http.Handler that will handle requests to this endpoint.
	Handler http.Handler
}

func (e Endpoint) clone() Endpoint {
	return Endpoint{
		Method:  e.Method,
		Path:    e.Path,
		Handler: e.Handler,
	}
}

// EndpointsWithMiddleware applies the given middleware to all provided
// endpoints. It returns a new slice of Endpoints with the middleware applied to
// their handlers. The original endpoints are not modified.
func EndpointsWithMiddleware(middleware MiddlewareFunc, endpoints ...Endpoint) []Endpoint {
	if middleware == nil {
		return endpoints
	}

	epts := make([]Endpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		e := endpoint.clone()
		e.Handler = middleware(e.Handler)
		epts = append(epts, e)
	}

	return epts
}

// EndpointsWithPrefix prefixes the given path to all provided endpoints. It
// returns a new slice of Endpoints with the prefixed paths. The original
// endpoints are not modified.
func EndpointsWithPrefix(prefix string, endpoints ...Endpoint) []Endpoint {
	epts := make([]Endpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		e := endpoint.clone()
		e.Path = prefix + endpoint.Path
		epts = append(epts, e)
	}

	return epts
}
