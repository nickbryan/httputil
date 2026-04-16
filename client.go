package httputil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Client is an HTTP client that wraps a standard http.Client and provides
// convenience methods for making requests and handling responses.
type Client struct {
	basePath string
	client   *http.Client
	encoder  ClientEncoder
}

// NewClient creates a new Client with the given options.
func NewClient(options ...ClientOption) *Client {
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
		encoder: opts.encoder,
	}
}

// BasePath returns the base path for the Client.
func (c *Client) BasePath() string {
	return c.basePath
}

// Client returns the underlying *http.Client.
func (c *Client) Client() *http.Client {
	return c.client
}

// Do executes the provided request using the Client's underlying *http.Client.
// Unlike the method helpers (Get, Post, etc.), Do does not prepend BasePath;
// the caller is responsible for constructing the full URL.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req) //nolint:wrapcheck,gosec // No additional context to add; G704: caller controls request URL (standard http.Client.Do semantics).
}

// Get sends an HTTP GET request to the specified path.
func (c *Client) Get(ctx context.Context, path string, options ...RequestOption) (*http.Response, error) {
	return c.do(ctx, http.MethodGet, path, nil, options...)
}

// Post sends an HTTP POST request to the specified path with the given body.
func (c *Client) Post(ctx context.Context, path string, body any, options ...RequestOption) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, path, body, options...)
}

// Put sends an HTTP PUT request to the specified path with the given body.
func (c *Client) Put(ctx context.Context, path string, body any, options ...RequestOption) (*http.Response, error) {
	return c.do(ctx, http.MethodPut, path, body, options...)
}

// Patch sends an HTTP PATCH request to the specified path with the given body.
func (c *Client) Patch(ctx context.Context, path string, body any, options ...RequestOption) (*http.Response, error) {
	return c.do(ctx, http.MethodPatch, path, body, options...)
}

// Delete sends an HTTP DELETE request to the specified path.
func (c *Client) Delete(ctx context.Context, path string, options ...RequestOption) (*http.Response, error) {
	return c.do(ctx, http.MethodDelete, path, nil, options...)
}

// do executes an HTTP request with the given method, path, body, and options.
func (c *Client) do(ctx context.Context, method, path string, body any, options ...RequestOption) (*http.Response, error) {
	opts := mapRequestOptionsToDefaults(options)

	reqURL, err := url.JoinPath(c.BasePath(), path)
	if err != nil {
		return nil, fmt.Errorf("building request url: %w", err)
	}

	var bodyReader io.Reader

	if body != nil {
		if reader, ok := body.(io.Reader); ok {
			bodyReader = reader
		} else {
			reader, err = c.encoder.Encode(body)
			if err != nil {
				return nil, fmt.Errorf("encoding request body: %w", err)
			}

			bodyReader = reader
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.URL.RawQuery = opts.params.Encode()
	req.Header = opts.header

	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", c.encoder.ContentType())
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return resp, nil
}
