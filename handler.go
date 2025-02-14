package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/nickbryan/httputil/problem"
)

type (
	// Handler defines the interface for a handler function. It takes a Request that
	// has data of type D and returns a Response or an error.
	Handler[D, P any] func(r Request[D, P]) (*Response, error)

	// Request represents an HTTP request that expects Prams and Data.
	Request[D, P any] struct {
		*http.Request
		Data           D
		Params         P
		ResponseWriter http.ResponseWriter
	}

	// RequestData represents a Request that expects Data but no Params.
	RequestData[D any] = Request[D, struct{}]

	// RequestEmpty represents an empty Request that expects no Prams or Data.
	RequestEmpty = Request[struct{}, struct{}]

	// RequestParams represents a Request that expects Params but no Data.
	RequestParams[P any] = Request[struct{}, P]

	// Response represents an HTTP response that holds optional data and the
	// required information to write a response.
	Response struct {
		code     int
		data     any
		redirect string
	}

	// Interceptor allows for operations to be performed on the
	// Request, Response or Params data before it gets finalized.
	Interceptor interface {
		Intercept(ctx context.Context) error
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
// indicate to the Handler that a redirect should be written.
func Redirect(code int, url string) (*Response, error) {
	return &Response{
		code:     code,
		data:     nil,
		redirect: url,
	}, nil
}

type jsonHandler[D, P any] struct {
	handler                             Handler[D, P]
	logger                              *slog.Logger
	reqIsStructType, paramsIsStructType bool
}

// NewJSONHandler creates a new http.Handler that wraps the provided [Handler]
// function to deserialize JSON request bodies and serialize JSON response
// bodies.
func NewJSONHandler[D, P any](handler Handler[D, P]) http.Handler {
	return &jsonHandler[D, P]{
		handler: handler,
		logger:  nil,
		// Cache this early as reflection can be expensive.
		reqIsStructType:    reflect.TypeFor[D]().Kind() == reflect.Struct,
		paramsIsStructType: reflect.TypeFor[P]().Kind() == reflect.Struct,
	}
}

// setLogger is used by the server to inject the logger that will be used by the handler.
func (h *jsonHandler[D, P]) setLogger(l *slog.Logger) { h.logger = l }

// ServeHTTP implements the http.Handler interface. It reads the request body,
// decodes it into the request data, validates it if a validator is set, calls
// the wrapped handler, and writes the response back in JSON format.
func (h *jsonHandler[D, P]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	//nolint:exhaustruct // Zero value of Data is unknown.
	request, ok := h.processRequest(Request[D, P]{Request: r, ResponseWriter: w})
	if !ok {
		return
	}

	response, err := h.handler(request)
	if err != nil {
		h.writeErrorResponse(r.Context(), w, err)
		return
	}

	h.processResponse(request, response)
}

func (h *jsonHandler[D, P]) processRequest(req Request[D, P]) (Request[D, P], bool) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to read request body", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return req, false
	}

	if !isEmptyStruct(req.Params) {
		if err = decodeParams(req.Request, &req.Params); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to decode params data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.BadRequest(req.Request)) // TODO: need custom errors for param validation.
		}

		if h.paramsIsStructType {
			if err = validate.StructCtx(req.Context(), &req.Params); err != nil {
				h.writeValidationErr(req.ResponseWriter, req.Request, err) // TODO: need custom errors for param validation.
				return req, false
			}
		}

		if err = intercept(req.Context(), &req.Params); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to intercept params data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

			return req, false
		}
	}

	if !isEmptyStruct(req.Data) {
		if len(body) == 0 {
			h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.BadRequest(req.Request).WithDetail("The server received an unexpected empty request body"))
			return req, false
		}

		if err = json.Unmarshal(body, &req.Data); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to decode request data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.BadRequest(req.Request))

			return req, false
		}

		if h.reqIsStructType {
			if err = validate.StructCtx(req.Context(), &req.Data); err != nil {
				h.writeValidationErr(req.ResponseWriter, req.Request, err)
				return req, false
			}
		}

		if err = intercept(req.Context(), &req.Data); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to intercept request data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

			return req, false
		}
	}

	// Put the body contents back so that it can be read in the handler again if
	// desired. We have consumed the buffer when reading Body above.
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return req, true
}

func (h *jsonHandler[D, P]) processResponse(req Request[D, P], res *Response) {
	if res == nil {
		return // TODO: test this, to allow for the underlying writer to be used instead
	}

	if res.redirect != "" { // TODO: test this
		http.Redirect(req.ResponseWriter, req.Request, res.redirect, res.code)
		return
	}

	req.ResponseWriter.WriteHeader(res.code)

	if res.data == nil {
		return
	}

	if err := intercept(req.Context(), &res.data); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to intercept res data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return
	}

	h.writeResponse(req.Context(), req.ResponseWriter, res.data)
}

func (h *jsonHandler[D, P]) writeValidationErr(w http.ResponseWriter, r *http.Request, err error) {
	// This should never really happen as we validate if the expected request.Data is
	// a struct which is a valid value for StructCtx. This error only gets returned
	// on invalid types being passed to `Struct`, `StructExcept`, StructPartial` or
	// `Property` and their context variants. This means there is unfortunately no way
	// to test this.
	var invalidValidationError *validator.InvalidValidationError
	if errors.As(err, &invalidValidationError) {
		h.logger.ErrorContext(r.Context(), "JSON handler failed to validate request data", slog.Any("error", err))
		h.writeErrorResponse(r.Context(), w, problem.ServerError(r))

		return
	}

	var errs validator.ValidationErrors
	if errors.As(err, &errs) {
		fields := make([]problem.Property, 0, len(errs))
		for _, err := range errs {
			fields = append(fields, problem.Property{Detail: explainValidationError(err), Pointer: "/" + strings.Join(strings.Split(err.Namespace(), ".")[1:], "/")})
		}

		h.writeErrorResponse(r.Context(), w, problem.ConstraintViolation(r, fields...))

		return
	}

	// The validator should never return an unknown error type based in its current
	// implementation, but we handle it anyway in case that ever changes.
	// Unfortunately, like above, there is no way to test this.
	h.logger.ErrorContext(r.Context(), "JSON handler received an unknown validation error", slog.Any("error", err))
	h.writeErrorResponse(r.Context(), w, problem.ServerError(r))
}

func (h *jsonHandler[D, P]) writeErrorResponse(ctx context.Context, w http.ResponseWriter, err error) {
	var problemDetails *problem.DetailedError
	if errors.As(err, &problemDetails) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(problemDetails.Status)
		h.writeResponse(ctx, w, problemDetails)

		return
	}

	h.logger.ErrorContext(ctx, "JSON handler received an unhandled error from inner handler", slog.Any("error", err))
	w.WriteHeader(http.StatusInternalServerError)
}

func (h *jsonHandler[D, P]) writeResponse(ctx context.Context, w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(&data); err != nil {
		h.logger.ErrorContext(ctx, "JSON handler failed to encode response data", slog.Any("error", err))
	}
}

func isEmptyStruct(v any) bool {
	_, ok := v.(struct{})
	return ok
}

func intercept(ctx context.Context, data any) error {
	if interceptor, ok := data.(Interceptor); ok {
		if err := interceptor.Intercept(ctx); err != nil {
			return fmt.Errorf("intercepting data: %w", err)
		}
	}

	return nil
}
