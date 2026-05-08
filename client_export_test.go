package httputil

import "net/http"

// ClientHTTPClient returns the underlying *http.Client for use in tests.
// It exists so that option defaults and wiring can be asserted directly without
// exposing the field on the public API.
func ClientHTTPClient(c *Client) *http.Client {
	return c.client
}
