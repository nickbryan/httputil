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

func (c *Client) Get(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodGet, path, nil, options...)
}

func (c *Client) Post(ctx context.Context, path string, body any, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodPost, path, body, options...)
}

func (c *Client) Put(ctx context.Context, path string, body any, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodPut, path, body, options...)
}

func (c *Client) Patch(ctx context.Context, path string, body any, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodPatch, path, body, options...)
}

func (c *Client) Delete(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return c.do(ctx, http.MethodDelete, path, nil, options...)
}

func (c *Client) do(ctx context.Context, method, path string, body any, options ...RequestOption) (*Result, error) {
	opts := mapRequestOptionsToDefaults(options)

	reqURL, err := url.JoinPath(c.BasePath(), path)
	if err != nil {
		return nil, fmt.Errorf("building request url: %w", err)
	}

	var bodyReader io.Reader

	if body != nil {
		reader, err := c.codec.Encode(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}

		bodyReader = reader
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

type Result struct {
	*http.Response
	codec ClientCodec
}

func (cr *Result) AsProblemDetails() (*problem.DetailedError, error) {
	var problemDetails *problem.DetailedError

	if err := cr.Decode(&problemDetails); err != nil {
		return nil, err
	}

	return problemDetails, nil
}

func (cr *Result) IsError() bool {
	return cr.StatusCode >= 400
}

func (cr *Result) IsSuccess() bool {
	return cr.StatusCode > 199 && cr.StatusCode < 300
}

func (cr *Result) Decode(into any) (err error) {
	defer func(Body io.ReadCloser) {
		if e := Body.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("closing response body: %w", e))
		}
	}(cr.Body)

	return cr.codec.Decode(cr.Body, into)
}
