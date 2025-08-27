// Package httputil provides utilities for building HTTP clients and servers.
package httputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nickbryan/httputil/problem"
)

type (
	// Client is an HTTP client that wraps a standard http.Client and provides
	// convenience methods for making requests and handling responses.
	Client struct {
		basePath string
		client   *http.Client
		codec    ClientCodec
	}
)

// NewClient creates a new Client with the given options.
func NewClient(options ...ClientOption) *Client {
	opts := mapClientOptionsToDefaults(options)

	return &Client{
		basePath: strings.TrimRight(opts.basePath, "/"),
		client: &http.Client{
			CheckRedirect: opts.checkRedirect,
			Jar:           opts.jar,
			Timeout:       opts.timeout,
			Transport:     opts.transport,
		},
		codec: opts.codec,
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

// Ensure Client implements the ability to close inline with io.Closer.
var _ io.Closer = &Client{}

// Close closes any connections on its [http.Client.Transport] which were
// previously connected from previous requests but are now sitting idle in a
// "keep-alive" state. It does not interrupt any connections currently in use.
// See the [http.Client.CloseIdleConnections] documentation for details.
//
// If [http.Client.Transport] does not have a [http.Client.CloseIdleConnections]
// method then this method does nothing. Interestingly, the [http.Client] type
// does not implement the [io.Closer] interface. WithClientInterceptor wraps
// the [http.Client.Transport] to ensure that the CloseIdleConnections method
// is called.
func (c *Client) Close() error {
	c.client.CloseIdleConnections()

	return nil
}

// Do executes the provided request using the Client's underlying *http.Client.
// It returns the raw *http.Response and an error, if any.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

// Get sends an HTTP GET request to the specified path.
// It returns a Result which wraps the http.Response, or an error.
func (c *Client) Get(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodGet, path, nil, options...)
}

// Post sends an HTTP POST request to the specified path with the given body.
// It returns a Result which wraps the http.Response, or an error.
func (c *Client) Post(ctx context.Context, path string, body any, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodPost, path, body, options...)
}

// Put sends an HTTP PUT request to the specified path with the given body.
// It returns a Result which wraps the http.Response, or an error.
func (c *Client) Put(ctx context.Context, path string, body any, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodPut, path, body, options...)
}

// Patch sends an HTTP PATCH request to the specified path with the given body.
// It returns a Result which wraps the http.Response, or an error.
func (c *Client) Patch(ctx context.Context, path string, body any, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodPatch, path, body, options...)
}

// Delete sends an HTTP DELETE request to the specified path.
// It returns a Result which wraps the http.Response, or an error.
func (c *Client) Delete(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodDelete, path, nil, options...)
}

// do executes an HTTP request with the given method, path, body, and options.
// It handles request creation, body encoding, and response wrapping in a Result.
func (c *Client) do(ctx context.Context, method, path string, body any, options ...RequestOption) (*Result, error) {
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
			reader, err = c.codec.Encode(body)
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

	req.Header.Set("Accept", c.codec.ContentType())
	req.Header.Set("Content-Type", c.codec.ContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return &Result{
		Response: resp,
		codec:    c.codec,
	}, nil
}

// Result wraps an http.Response and provides convenience methods for
// decoding the response body and checking status codes.
type Result struct {
	*http.Response
	codec ClientCodec
}

// AsProblemDetails attempts to decode the response body into a
// problem.DetailedError. This is useful for handling API errors that conform to
// RFC 7807.
//
// Note: This method consumes the response body. Subsequent calls to
// Decode or AsProblemDetails will fail if the body has already been read.
func (r *Result) AsProblemDetails() (*problem.DetailedError, error) {
	var problemDetails *problem.DetailedError

	if err := r.Decode(&problemDetails); err != nil {
		return nil, err
	}

	return problemDetails, nil
}

// IsError returns true if the HTTP status code is 400 or greater.
func (r *Result) IsError() bool {
	return r.StatusCode >= 400
}

// IsSuccess returns true if the HTTP status code is between 200 and 299 (inclusive).
func (r *Result) IsSuccess() bool {
	return r.StatusCode > 199 && r.StatusCode < 300
}

// Decode decodes the response body into the provided target. It uses the
// ClientCodec to perform the decoding.
//
// Note: This method consumes the response body. Subsequent calls to Decode or
// AsProblemDetails will fail if the body has already been read.
func (r *Result) Decode(into any) (err error) {
	defer func(Body io.ReadCloser) {
		if e := Body.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("closing response body: %w", e))
		}
	}(r.Body)

	return r.codec.Decode(r.Body, into)
}
