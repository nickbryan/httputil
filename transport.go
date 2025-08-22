package httputil

import (
	"net/http"
)

// InterceptorFunc defines a function type for HTTP client middleware. An InterceptorFunc
// takes an http.RoundTripper as input and returns a new http.RoundTripper that wraps the
// original action with additional logic.
type InterceptorFunc func(next http.RoundTripper) http.RoundTripper

// RoundTripperFunc is an adapter to allow the use of ordinary functions as
// http.RoundTripper. If f is a function with the appropriate signature,
// RoundTripperFunc(f) is a http.RoundTripper that calls f.
type RoundTripperFunc func(req *http.Request) (*http.Response, error)

// Ensure that RoundTripperFunc implements the http.RoundTripper interface.
var _ http.RoundTripper = RoundTripperFunc(nil)

// RoundTrip implements the http.RoundTripper interface.
func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// closeIdleConnectionsPropagatingRoundTripper is an http.RoundTripper middleware
// that ensures calls to http.Client.CloseIdleConnections are propagated to the
// underlying transport.
//
// The http.RoundTripper interface does not include the CloseIdleConnections method.
// Instead, the http.Client uses a type assertion to check if its Transport
// implements the method. This wrapper ensures that custom transport chains
// don't break this behavior.
type closeIdleConnectionsPropagatingRoundTripper struct {
	next http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface for this type and acts as a
// pass-through to the underlying transport.
func (rt closeIdleConnectionsPropagatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt.next.RoundTrip(req)
}

// CloseIdleConnections propagates the call to CloseIdleConnections to the underlying
// transport.
func (rt closeIdleConnectionsPropagatingRoundTripper) CloseIdleConnections() {
	if n, ok := rt.next.(interface{ CloseIdleConnections() }); ok {
		n.CloseIdleConnections()
	}
}

// newCloseIdleConnectionsPropagatingRoundTripper returns a new
// closeIdleConnectionsPropagatingRoundTripper that wraps the given
// http.RoundTripper. It is used by the WithClientInterceptor option.
func newCloseIdleConnectionsPropagatingRoundTripper(next http.RoundTripper) http.RoundTripper {
	return closeIdleConnectionsPropagatingRoundTripper{next}
}
