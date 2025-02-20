package httputil

import (
	"net/http"
)

type (
	// Endpoint represents a registered HTTP endpoint.
	Endpoint struct {
		// Method is the HTTP method for this endpoint (e.g., "GET", "POST", "PUT", "DELETE").
		Method string
		// Path is the URL path for this endpoint (e.g., "/users", "/products/{id}").
		Path string
		// Handler is the http.Handler that will handle requests to this endpoint.
		Handler http.Handler

		requestInterceptor RequestInterceptor
	}

	// TODO: where does this belong, it will not be called when the handler is just a http.Handler?
	RequestInterceptor interface {
		InterceptRequest(w http.ResponseWriter, r *http.Request) (*Response, error)
	}

	RequestInterceptorStack []RequestInterceptor
)

func (s RequestInterceptorStack) InterceptRequest(w http.ResponseWriter, r *http.Request) (*Response, error) {
	for _, i := range s {
		if response, err := i.InterceptRequest(w, r); response != nil || err != nil {
			return response, err
		}
	}

	return nil, nil
}

// EndpointsWithRequestInterceptor sets the RequestInterceptor on each Endpoint. It
// returns a new slice of Endpoints with the RequestInterceptor set. The original
// endpoints are not modified.
func EndpointsWithRequestInterceptor(interceptor RequestInterceptor, endpoints ...Endpoint) []Endpoint {
	return cloneAndUpdate(endpoints, func(e *Endpoint) {
		e.requestInterceptor = interceptor
	})
}

// EndpointsWithMiddleware applies the given middleware to all provided
// endpoints. It returns a new slice of Endpoints with the middleware applied to
// their handlers. The original endpoints are not modified.
func EndpointsWithMiddleware(middleware MiddlewareFunc, endpoints ...Endpoint) []Endpoint {
	return cloneAndUpdate(endpoints, func(e *Endpoint) {
		e.Handler = middleware(e.Handler)
	})
}

// EndpointsWithPrefix prefixes the given path to all provided endpoints. It
// returns a new slice of Endpoints with the prefixed paths. The original
// endpoints are not modified.
func EndpointsWithPrefix(prefix string, endpoints ...Endpoint) []Endpoint {
	return cloneAndUpdate(endpoints, func(e *Endpoint) {
		e.Path = prefix + e.Path
	})
}

func cloneAndUpdate(endpoints []Endpoint, update func(e *Endpoint)) []Endpoint {
	epts := make([]Endpoint, 0, len(endpoints))

	for _, endpoint := range endpoints {
		e := Endpoint{
			Method:  endpoint.Method,
			Path:    endpoint.Path,
			Handler: endpoint.Handler,
		}
		update(&e)
		epts = append(epts, e)
	}

	return epts
}
