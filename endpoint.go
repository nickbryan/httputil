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

	// EndpointGroup represents a group of Endpoint definitions
	// allowing access to helper functions to define the group.
	EndpointGroup []Endpoint

	// TODO: where does this belong, it will not be called when the handler is just a http.Handler?
	RequestInterceptor interface {
		InterceptRequest(w http.ResponseWriter, r *http.Request) (*Response, error)
	}

	RequestInterceptorStack []RequestInterceptor
)

func (s RequestInterceptorStack) InterceptRequest(w http.ResponseWriter, r *http.Request) (*Response, error) {
	for _, i := range s {
		if response, err := i.InterceptRequest(w, r); response != nil || err != nil {
			return response, err //nolint:nilnil,wrapcheck // Request intercepted allow interceptor to determine result.
		}
	}

	return nil, nil //nolint:nilnil // nil, nil signals continue.
}

// WithRequestInterceptor sets the RequestInterceptor on each Endpoint. It
// returns a new slice of EndpointGroup with the RequestInterceptor set. The original
// endpoints are not modified.
func (eg EndpointGroup) WithRequestInterceptor(interceptor RequestInterceptor) EndpointGroup {
	if interceptor == nil {
		return eg
	}

	return cloneAndUpdate(eg, func(e *Endpoint) {
		e.requestInterceptor = interceptor
	})
}

// WithMiddleware applies the given middleware to all provided
// endpoints. It returns a new slice of EndpointGroup with the middleware applied to
// their handlers. The original endpoints are not modified.
func (eg EndpointGroup) WithMiddleware(middleware MiddlewareFunc) EndpointGroup {
	if middleware == nil {
		return eg
	}

	return cloneAndUpdate(eg, func(e *Endpoint) {
		e.Handler = middleware(e.Handler)
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
