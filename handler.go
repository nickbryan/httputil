package httputil

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

type (
	// Action defines the interface for an action function that will be called by
	// the handler. It takes a Request that has data of type D and params of type P
	// and returns a Response or an error.
	Action[D, P any] func(r Request[D, P]) (*Response, error)

	// Handler represents an interface that combines HTTP handling and additional
	// guard and logging functionality ensuring that dependencies can be
	// passed through to the handler.
	Handler interface {
		with(l *slog.Logger, g Guard) Handler
		http.Handler
	}

	// Guard defines an interface for components that protect access to a Handler's
	// Action. It acts as a crucial pre-processing gatekeeper within the handler's
	// request lifecycle, executing after the request has been routed to the handler
	// but *before* any automatic request body decoding or parameter binding occurs.
	//
	// Its primary role is to enforce preconditions, such as authentication,
	// authorization, API key validation, or other checks based on request headers,
	// context, or basic properties, before allowing the request to proceed to the
	// core business logic (the Action).
	Guard interface {
		Guard(r *http.Request) (*http.Request, error)
	}

	// Transformer allows for operations to be performed on the Request, Response or
	// Params data before it gets finalized. A Transformer will not be called for a
	// standard http.Handler as there is nothing to transform.
	Transformer interface {
		Transform(ctx context.Context) error
	}
)

type (
	// Request is a generic HTTP request wrapper that contains request data,
	// parameters, and a response writer.
	Request[D, P any] struct {
		*http.Request
		// Data holds the request-specific data of generic type D, which is provided
		// when initializing the request. A [Handler] will attempt to decode the Request
		// body into this type.
		Data D
		// Params holds the parameters of generic type P associated with the request,
		// allowing dynamic decoding and validation of Request parameters. See
		// [BindValidParameters] documentation for usage information.
		Params P
		// ResponseWriter is an embedded HTTP response writer used to construct and send
		// the HTTP response. When writing a response via the ResponseWriter directly it
		// is best practice to return a [NothingToHandle] response so that the handler
		// does not try to encode response data or handle errors.
		ResponseWriter http.ResponseWriter
	}

	// RequestData represents a Request that expects data but no Params.
	RequestData[D any] = Request[D, struct{}]

	// RequestEmpty represents an empty Request that expects no Prams or data.
	RequestEmpty = Request[struct{}, struct{}]

	// RequestParams represents a Request that expects Params but no data.
	RequestParams[P any] = Request[struct{}, P]

	// Response represents an HTTP response that holds optional data and the
	// required information to write a response.
	Response struct {
		code     int
		data     any
		redirect string
	}
)

// GuardFunc is a function type for modifying or inspecting an HTTP
// request, potentially returning an altered request. This is useful for
// authentication and adding claims to the request context.
type GuardFunc func(r *http.Request) (*http.Request, error)

// Guard applies the GuardFunc to modify or inspect the provided HTTP request.
func (rif GuardFunc) Guard(r *http.Request) (*http.Request, error) {
	return rif(r)
}

// NewResponse creates a new Response object with the given status code and data.
func NewResponse(code int, data any) *Response {
	return &Response{
		code:     code,
		data:     data,
		redirect: "",
	}
}

// Accepted creates a new Response object with a status code of
// http.StatusAccepted (202 Accepted) and the given data.
func Accepted(data any) (*Response, error) {
	return &Response{
		code:     http.StatusAccepted,
		data:     data,
		redirect: "",
	}, nil
}

// Created creates a new Response object with a status code of
// http.StatusCreated (201 Created) and the given data.
func Created(data any) (*Response, error) {
	return &Response{
		code:     http.StatusCreated,
		data:     data,
		redirect: "",
	}, nil
}

// NoContent creates a new Response object with a status code of
// http.StatusNoContent (204 No Content) and an empty struct as data.
func NoContent() (*Response, error) {
	return &Response{
		code:     http.StatusNoContent,
		data:     nil,
		redirect: "",
	}, nil
}

// NothingToHandle returns a nil Response and a nil error, intentionally
// representing a scenario with no response output so the Handler does not
// attempt to process a response. This adds clarity when a Guard
// does not block the request or when acting on Request.ResponseWriter directly.
func NothingToHandle() (*Response, error) {
	return nil, nil //nolint:nilnil // Intentional.
}

// OK creates a new Response with HTTP status code 200 (OK) containing the
// provided data.
func OK(data any) (*Response, error) {
	return &Response{
		code:     http.StatusOK,
		data:     data,
		redirect: "",
	}, nil
}

// Redirect creates a new Response object with the given status code
// and an empty struct as data. The redirect url will be set which will
// indicate to the handler that a redirect should be written.
func Redirect(code int, url string) (*Response, error) {
	return &Response{
		code:     code,
		data:     nil,
		redirect: url,
	}, nil
}

// transform applies a transformation to the given data if it implements the
// Transformer interface. Returns an error if the transformation fails;
// otherwise, returns nil. Context is used to manage request-scoped values and
// cancellation.
func transform(ctx context.Context, data any) error {
	if transformer, ok := data.(Transformer); ok {
		if err := transformer.Transform(ctx); err != nil {
			return fmt.Errorf("transforming data: %w", err)
		}
	}

	return nil
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}

	_, ok := v.(struct{})

	return ok
}
