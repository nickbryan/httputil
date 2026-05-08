package httputil

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

// Client is an HTTP client that wraps a standard http.Client and provides
// convenience methods for making requests and handling responses.
type Client struct {
	basePath     string
	client       *http.Client
	codec        ClientCodec
	errorDecoder ErrorDecoder
	logger       *slog.Logger
}

// NewClient creates a new Client with the given options.
func NewClient(logger *slog.Logger, options ...ClientOption) *Client {
	opts := mapClientOptionsToDefaults(options)

	transport := opts.rootTransport
	for _, intercept := range slices.Backward(opts.interceptors) {
		transport = intercept(transport)
	}

	return &Client{
		basePath: strings.TrimRight(opts.basePath, "/"),
		client: &http.Client{
			CheckRedirect: opts.checkRedirect,
			Jar:           opts.jar,
			Timeout:       opts.timeout,
			Transport:     transport,
		},
		codec:        opts.codec,
		errorDecoder: opts.errorDecoder,
		logger:       logger,
	}
}

// BasePath returns the base path for the Client.
func (c *Client) BasePath() string {
	return c.basePath
}

// Codec returns the ClientCodec configured on the Client.
func (c *Client) Codec() ClientCodec { //nolint:ireturn // Intentional interface return for codec abstraction.
	return c.codec
}

// Do executes the provided request using the Client's underlying *http.Client.
// Unlike the package-level generic functions (Get, Post, etc.), Do does not
// prepend BasePath; the caller is responsible for constructing the full URL.
//
// Do implements the http.RoundTripper-adjacent single-method interface expected
// by HTTP client wrappers, allowing *Client to be passed directly to libraries
// that accept a Do(req *http.Request) (*http.Response, error) doer so that all
// configured interceptors are applied automatically.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req) //nolint:wrapcheck // No additional context to add.
}
