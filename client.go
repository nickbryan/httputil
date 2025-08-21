package httputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nickbryan/httputil/problem"
)

type Client struct {
	basePath string
	client   *http.Client
	codec    ClientCodec
}

func NewClient(options ...ClientOption) *Client {
	opts := mapClientOptionsToDefaults(options)

	return &Client{
		basePath: strings.TrimRight(opts.basePath, "/"),
		client: &http.Client{
			CheckRedirect: opts.checkRedirect,
			Jar:           opts.jar,
			Timeout:       opts.timeout,
		},
		codec: opts.codec,
	}
}

// BasePath returns the base path for the Client.
func (c *Client) BasePath() string {
	return c.basePath
}

// WrappedClient provides access to the underlying *http.Client.
func (c *Client) WrappedClient() *http.Client {
	return c.client
}

// Do executes the provided request using the Client's underlying *http.Client.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

func (c *Client) Get(ctx context.Context, path string, into any, options ...RequestOption) (resp *http.Response, err error) {
	return c.do(ctx, http.MethodGet, path, nil, into, options...)
}

func (c *Client) Post(ctx context.Context, path string, body, into any, options ...RequestOption) (resp *http.Response, err error) {
	return c.do(ctx, http.MethodPost, path, body, into, options...)
}

func (c *Client) Put(ctx context.Context, path string, body, into any, options ...RequestOption) (resp *http.Response, err error) {
	return c.do(ctx, http.MethodPut, path, body, into, options...)
}

func (c *Client) Patch(ctx context.Context, path string, body, into any, options ...RequestOption) (resp *http.Response, err error) {
	return c.do(ctx, http.MethodPatch, path, body, into, options...)
}

func (c *Client) Delete(ctx context.Context, path string, into any, options ...RequestOption) (resp *http.Response, err error) {
	return c.do(ctx, http.MethodDelete, path, nil, into, options...)
}

func (c *Client) do(ctx context.Context, method, path string, body, into any, options ...RequestOption) (resp *http.Response, err error) {
	opts := mapRequestOptionsToDefaults(options)

	path = strings.TrimRight(c.BasePath(), "/") + "/" + strings.TrimLeft(path, "/")

	var bodyReader io.Reader
	if body != nil {
		bodyReader, err = c.codec.Encode(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.URL.RawQuery = opts.params.Encode()
	req.Header = opts.header

	resp, err = c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	defer func(Body io.ReadCloser) {
		if e := Body.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("closing response body: %w", e))
		}
	}(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, &UnsuccessfulRequestError{
			codec:    c.codec,
			Response: resp,
		}
	}

	if into != nil {
		if err = c.codec.Decode(resp.Body, into); err != nil {
			return nil, fmt.Errorf("decoding response result: %w", err)
		}
	}

	return resp, nil
}

type ClientResponse[T any] struct {
	*http.Response
	Data T
}

// Get performs a GET request using the provided Client and decodes the response
// body into the provided target type.
func Get[T any](ctx context.Context, client *Client, path string, options ...RequestOption) (*ClientResponse[T], error) {
	var responseData T

	resp, err := client.Get(ctx, path, &responseData, options...)
	if err != nil {
		return nil, err
	}

	return &ClientResponse[T]{
		Response: resp,
		Data:     responseData,
	}, nil
}

// Post performs a POST request using the provided Client and decodes the response
// body into the provided target type.
func Post[T any](ctx context.Context, client *Client, path string, body any, options ...RequestOption) (*ClientResponse[T], error) {
	var responseData T

	resp, err := client.Post(ctx, path, body, &responseData, options...)
	if err != nil {
		return nil, err
	}

	return &ClientResponse[T]{
		Response: resp,
		Data:     responseData,
	}, nil
}

// Put performs a PUT request using the provided Client and decodes the response
// body into the provided target type.
func Put[T any](ctx context.Context, client *Client, path string, body any, options ...RequestOption) (*ClientResponse[T], error) {
	var responseData T

	resp, err := client.Put(ctx, path, body, &responseData, options...)
	if err != nil {
		return nil, err
	}

	return &ClientResponse[T]{
		Response: resp,
		Data:     responseData,
	}, nil
}

// Patch performs a PATCH request using the provided Client and decodes the response
// body into the provided target type.
func Patch[T any](ctx context.Context, client *Client, path string, body any, options ...RequestOption) (*ClientResponse[T], error) {
	var responseData T

	resp, err := client.Patch(ctx, path, body, &responseData, options...)
	if err != nil {
		return nil, err
	}

	return &ClientResponse[T]{
		Response: resp,
		Data:     responseData,
	}, nil
}

// Delete performs a DELETE request using the provided Client and decodes the response
// body into the provided target type.
func Delete[T any](ctx context.Context, client *Client, path string, options ...RequestOption) (*ClientResponse[T], error) {
	var responseData T

	resp, err := client.Delete(ctx, path, &responseData, options...)
	if err != nil {
		return nil, err
	}

	return &ClientResponse[T]{
		Response: resp,
		Data:     responseData,
	}, nil
}

type UnsuccessfulRequestError struct {
	codec ClientCodec

	Response *http.Response
}

func (e *UnsuccessfulRequestError) Error() string {
	return "unsuccessful request"
}

func (e *UnsuccessfulRequestError) AsProblemDetails() (*problem.DetailedError, error) {
	var problemDetails *problem.DetailedError

	if err := e.Decode(&problemDetails); err != nil {
		return nil, err
	}

	return problemDetails, nil
}

func (e *UnsuccessfulRequestError) Decode(into any) error {
	if err := e.codec.Decode(e.Response.Body, into); err != nil {
		return fmt.Errorf("decoding response body: %w", err)
	}

	return nil
}