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
