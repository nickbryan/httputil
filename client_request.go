package httputil

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/nickbryan/httputil/problem"
)

// Result is the outcome of a successful request.
//
// The embedded response's Body has been drained and closed by the time
// a Result is returned — do not attempt to read from it.
type Result[T any] struct {
	*http.Response

	Data T
}

// Get sends an HTTP GET request and decodes the response into T. Use struct{}
// as the type parameter to skip response body decoding.
func Get[T any](ctx context.Context, client *Client, path string, opts ...RequestOption) (*Result[T], error) {
	return doRequest[T](ctx, client, http.MethodGet, path, nil, opts...)
}

// Post sends an HTTP POST request with the given body and decodes the response
// into T. Use struct{} as the type parameter to skip response body decoding.
func Post[T any](ctx context.Context, client *Client, path string, body any, opts ...RequestOption) (*Result[T], error) {
	return doRequest[T](ctx, client, http.MethodPost, path, body, opts...)
}

// Put sends an HTTP PUT request with the given body and decodes the response
// into T. Use struct{} as the type parameter to skip response body decoding.
func Put[T any](ctx context.Context, client *Client, path string, body any, opts ...RequestOption) (*Result[T], error) {
	return doRequest[T](ctx, client, http.MethodPut, path, body, opts...)
}

// Patch sends an HTTP PATCH request with the given body and decodes the response
// into T. Use struct{} as the type parameter to skip response body decoding.
func Patch[T any](ctx context.Context, client *Client, path string, body any, opts ...RequestOption) (*Result[T], error) {
	return doRequest[T](ctx, client, http.MethodPatch, path, body, opts...)
}

// Delete sends an HTTP DELETE request and decodes the response into T. Use
// struct{} as the type parameter to skip response body decoding. body may be
// nil for the common case where no request body is required.
func Delete[T any](ctx context.Context, client *Client, path string, body any, opts ...RequestOption) (*Result[T], error) {
	return doRequest[T](ctx, client, http.MethodDelete, path, body, opts...)
}

func doRequest[T any](ctx context.Context, client *Client, method, path string, body any, opts ...RequestOption) (*Result[T], error) {
	reqOpts := mapRequestOptionsToDefaults(opts)

	req, err := newRequest(ctx, client, method, path, body, reqOpts)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	defer func() {
		if resp.Body != nil {
			if _, drainErr := io.Copy(io.Discard, resp.Body); drainErr != nil {
				client.logger.ErrorContext(ctx, "Failed to drain response body", slog.Any("error", drainErr))
			}

			if closeErr := resp.Body.Close(); closeErr != nil {
				client.logger.ErrorContext(ctx, "Failed to close response body", slog.Any("error", closeErr))
			}
		}
	}()

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		var data T

		if !isEmpty(data) {
			if err = client.Codec().Decode(resp.Body, &data); err != nil {
				return nil, fmt.Errorf("decoding response body: %w", err)
			}
		}

		return &Result[T]{Data: data, Response: resp}, nil
	}

	decoder := reqOpts.errorDecoder
	if decoder == nil {
		decoder = client.errorDecoder
	}

	return nil, handleErrorResponse(client, resp, decoder)
}

func handleErrorResponse(client *Client, resp *http.Response, decoder ErrorDecoder) error {
	if decoder != nil {
		if resp.Body != nil {
			resp.Body = &idempotentCloser{ReadCloser: resp.Body}
		}

		if decodeErr := decoder(resp, client.Codec()); decodeErr != nil {
			return decodeErr
		}

		return &UnexpectedResponseError{Response: resp}
	}

	if IsProblem(resp) {
		pe := ProblemResponseError{
			Response: resp,
			Problem:  &problem.DetailedError{},
		}

		if err := client.Codec().Decode(resp.Body, pe.Problem); err != nil {
			return fmt.Errorf("decoding problem response: %w", err)
		}

		return &pe
	}

	return &UnexpectedResponseError{Response: resp}
}

// idempotentCloser wraps an io.ReadCloser so that subsequent Close calls are
// no-ops and reads after Close return io.EOF. This prevents errors when the
// deferred drain/close in doRequest runs after an ErrorDecoder has already
// closed the body.
type idempotentCloser struct {
	io.ReadCloser

	closed bool
}

func (c *idempotentCloser) Read(p []byte) (int, error) {
	if c.closed {
		return 0, io.EOF
	}

	return c.ReadCloser.Read(p) //nolint:wrapcheck // transparent wrapper; wrapping would break io.EOF sentinel checks
}

func (c *idempotentCloser) Close() error {
	if c.closed {
		return nil
	}

	c.closed = true

	return c.ReadCloser.Close() //nolint:wrapcheck // transparent wrapper; caller logs the error directly
}

func newRequest(ctx context.Context, client *Client, method, path string, body any, reqOpts requestOptions) (*http.Request, error) {
	parsedPath, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("parsing request path: %w", err)
	}

	reqURL, err := url.JoinPath(client.BasePath(), parsedPath.EscapedPath())
	if err != nil {
		return nil, fmt.Errorf("building request url: %w", err)
	}

	var bodyReader io.Reader

	if body != nil {
		if reader, ok := body.(io.Reader); ok {
			bodyReader = reader
		} else {
			reader, err = client.Codec().Encode(body)
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

	merged := parsedPath.Query()

	for k, vals := range reqOpts.params {
		for _, v := range vals {
			merged.Add(k, v)
		}
	}

	req.URL.RawQuery = merged.Encode()

	req.Header = reqOpts.header

	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", client.Codec().ContentType())
	}

	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", client.Codec().ContentType())
	}

	return req, nil
}
