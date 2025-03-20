// Package problemtest provides utilities for creating and testing problems.
package problemtest

import (
	"net/http"
	"net/http/httptest"
)

// NewRequest creates and returns a new request for the specified instance URL to be used in testing.
func NewRequest(instance string) *http.Request {
	return httptest.NewRequest(http.MethodGet, instance, nil)
}
