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
	// Handler defines the interface for a handler function. It takes a request of
	// type `req` and returns a response of type `res` along with any potential
	// error.
	Handler[req, res any] func(r Request[req]) (*Response[res], error)

	// Request represents an HTTP request with additional data of type `T`.
	Request[T any] struct {
		*http.Request
		Data T
	}

	// RequestNoBody represents an empty Request.
	RequestNoBody = Request[struct{}]

	// Response represents an HTTP response with data of type `T` and a status code.
	Response[T any] struct {
		Header http.Header
		data   T
		code   int
	}

	// ResponseNoBody represents an empty Response.
	ResponseNoBody = Response[struct{}]
)

// NewNoContentResponse creates a new Response object with a status code of
// http.StatusNoContent (204 No Content) and an empty struct as data.
func NewNoContentResponse() *ResponseNoBody {
	return &ResponseNoBody{
		Header: make(http.Header),
		data:   struct{}{},
		code:   http.StatusNoContent,
	}
}

// NewResponse creates a new Response object with the given status code and data.
func NewResponse[T any](code int, data T) *Response[T] {
	return &Response[T]{
		Header: make(http.Header),
		data:   data,
		code:   code,
	}
}

type jsonHandler[req, res any] struct {
	handler         Handler[req, res]
	logger          *slog.Logger
	reqIsStructType bool
}

// NewJSONHandler creates a new http.Handler that wraps the provided [Handler]
// function to deserialize JSON request bodies and serialize JSON response
// bodies.
func NewJSONHandler[req, res any](handler Handler[req, res]) http.Handler {
	return &jsonHandler[req, res]{
		handler: handler,
		logger:  nil,
		// Cache this early as reflection can be expensive.
		reqIsStructType: reflect.TypeFor[req]().Kind() == reflect.Struct,
	}
}

// setLogger is used by the server to inject the logger that will be used by the handler.
func (h *jsonHandler[req, res]) setLogger(l *slog.Logger) { h.logger = l }

// ServeHTTP implements the http.Handler interface. It reads the request body,
// decodes it into the request data, validates it if a validator is set, calls
// the wrapped handler, and writes the response back in JSON format.
func (h *jsonHandler[req, res]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request := Request[req]{Request: r} //nolint:exhaustruct // Zero value of Data is unknown.

	w.Header().Set("Content-Type", "application/json")

	body, err := io.ReadAll(request.Body)
	if err != nil {
		h.logger.WarnContext(r.Context(), "JSON handler failed to read request body", slog.Any("error", err))
		h.writeErrorResponse(r.Context(), w, problem.ServerError(r))

		return
	}

	if !isEmptyStruct(request.Data) {
		if len(body) == 0 {
			h.writeErrorResponse(r.Context(), w, problem.BadRequest(r).WithDetail("The server received an unexpected empty request body"))
			return
		}

		if err = json.Unmarshal(body, &request.Data); err != nil {
			h.logger.WarnContext(r.Context(), "JSON handler failed to decode request data", slog.Any("error", err))
			h.writeErrorResponse(r.Context(), w, problem.BadRequest(r))

			return
		}

		if h.reqIsStructType {
			if err = validate.StructCtx(r.Context(), &request.Data); err != nil {
				h.writeValidationErr(w, r, err)
				return
			}
		}

		// Put the body contents back so that it can be read in the handler again if
		// desired. We have consumed the buffer when reading Body above.
		request.Body = io.NopCloser(bytes.NewBuffer(body))
	}

	response, err := h.handler(request)
	if err != nil {
		h.writeErrorResponse(r.Context(), w, err)
		return
	}

	if response == nil {
		return // TODO: test this
	}

	writeHeaders(w, response.Header)
	w.WriteHeader(response.code)

	if !isEmptyStruct(response.data) {
		h.writeResponse(r.Context(), w, response.data)
	}
}

func explainValidationError(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", err.Field())
	case "email":
		return fmt.Sprintf("%s should be a valid email", err.Field())
	case "uuid":
		return fmt.Sprintf("%s should be a valid UUID", err.Field())
	case "e164":
		return fmt.Sprintf("%s should be a valid international phone number (e.g. +33 6 06 06 06 06)", err.Field())
	default:
		resp := fmt.Sprintf("%s should be %s", err.Field(), err.Tag())
		if err.Param() != "" {
			resp += "=" + err.Param()
		}
		return resp
	}
}

func (h *jsonHandler[req, res]) writeValidationErr(w http.ResponseWriter, r *http.Request, err error) {
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

func (h *jsonHandler[req, res]) writeErrorResponse(ctx context.Context, w http.ResponseWriter, err error) {
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

func (h *jsonHandler[req, res]) writeResponse(ctx context.Context, w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(&data); err != nil {
		h.logger.ErrorContext(ctx, "JSON handler failed to encode response data", slog.Any("error", err))
	}
}

func isEmptyStruct(v any) bool {
	_, ok := v.(struct{})
	return ok
}

func writeHeaders(w http.ResponseWriter, headers http.Header) {
	for header, values := range headers {
		for _, value := range values {
			w.Header().Add(header, value)
		}
	}
}
