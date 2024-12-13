package httputil

import (
	"net/http"
)

// TODO: do we validate this?
// Endpoint represents a registered HTTP endpoint.
type Endpoint struct {
	// Method is the HTTP method for this endpoint (e.g., "GET", "POST", "PUT", "DELETE").
	Method string
	// Path is the URL path for this endpoint (e.g., "/users", "/products/{id}").
	Path string
	// Handler is the http.Handler that will handle requests to this endpoint.
	Handler http.Handler
}

// EndpointsWithMiddleware applies the given middleware to all provided endpoints.
// It returns a new slice of Endpoints with the middleware applied to their handlers.
// The original endpoints are not modified.
func EndpointsWithMiddleware(middleware MiddlewareFunc, endpoints ...Endpoint) []Endpoint {
	epts := make([]Endpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		epts = append(epts, Endpoint{
			Method:  endpoint.Method,
			Path:    endpoint.Path,
			Handler: middleware(endpoint.Handler),
		})
	}

	return epts
}

// EndpointsWithPrefix prefixes the given path to all provided endpoints.
// It returns a new slice of Endpoints with the prefixed paths.
// The original endpoints are not modified.
func EndpointsWithPrefex(prefix string, endpoints ...Endpoint) []Endpoint {
	epts := make([]Endpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		epts = append(epts, Endpoint{
			Method:  endpoint.Method,
			Path:    prefix + endpoint.Path,
			Handler: endpoint.Handler,
		})
	}

	return epts
}
