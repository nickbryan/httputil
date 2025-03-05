package httputil

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
)

type (
	// Action defines the interface for an action function. It takes a Request that
	// has data of type D and params of type P and returns a Response or an error.
	Action[D, P any] func(r Request[D, P]) (*Response, error)

	// Guard will be called by the Handler before a request is handled to allow for
	// processes such as auth to run. A Guard will not be called for a standard
	// http.Handler.
	Guard interface {
		Guard(r *http.Request) (*Response, error)
	}

	// Handler wraps a http.Handler with the ability to initialize
	// the implementation with the Server logger and validator.
	Handler interface {
		init(l *slog.Logger, v *validator.Validate, g Guard)
		http.Handler
	}

	// Transformer allows for operations to be performed on the Request, Response or
	// Params data before it gets finalized. A Transformer will not be called for a
	// standard http.Handler.
	Transformer interface {
		Transform(ctx context.Context) error
	}
)

type (
	// Request represents an HTTP request that expects Prams and Data.
	Request[D, P any] struct {
		*http.Request
		Data           D
		Params         P
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

func transform(ctx context.Context, data any) error {
	if transformer, ok := data.(Transformer); ok {
		if err := transformer.Transform(ctx); err != nil {
			return fmt.Errorf("transforming data: %w", err)
		}
	}

	return nil
}

func isEmptyStruct(v any) bool {
	_, ok := v.(struct{})
	return ok
}
