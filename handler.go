package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/nickbryan/httputil/problem"
)

// TODO: how do we validate query params? path params?
type (
	// Handler defines the interface for a handler function. It takes a request of type `req`
	// and returns a response of type `res` along with any potential error.
	Handler[req, res any] func(r Request[req]) (*Response[res], error)

	// Request represents a HTTP request with additional data of type `T`.
	Request[T any] struct {
		*http.Request
		Data T
	}

	// Response represents a HTTP response with data of type `T` and a status code.
	Response[T any] struct {
		Header http.Header
		data   T
		code   int
	}
)

// NewResponse creates a new Response object with the given status code and data.
func NewResponse[T any](code int, data T) *Response[T] {
	return &Response[T]{
		Header: make(http.Header),
		data:   data,
		code:   code,
	}
}

// NewNoContentResponse creates a new Response object with a status code
// of http.StatusNoContent (204 No Content) and an empty struct as data.
func NewNoContentResponse() *Response[struct{}] {
	return &Response[struct{}]{
		Header: make(http.Header),
		data:   struct{}{},
		code:   http.StatusNoContent,
	}
}

// TODO: export type and drop new function?
// handlerError represents an error specific to the handler.
type handlerError struct {
	message string
	code    int
}

func (e *handlerError) Error() string {
	return e.message
}

// ErrInternal is a pre-defined handlerError representing an internal server error.
var ErrInternal = &handlerError{
	message: "internal server error",
	code:    http.StatusInternalServerError,
}

// NewHandlerError creates a new handlerError with the given message and status code.
func NewHandlerError(code int, message string) error {
	return &handlerError{
		message: message,
		code:    code,
	}
}

// NewJSONHandler creates a new http.Handler that wraps the provided [Handler] function
// to deserialize JSON request bodies and serialize JSON response bodies.
func NewJSONHandler[req, res any](handler Handler[req, res]) http.Handler {
	return &jsonHandler[req, res]{
		handler:   handler,
		logger:    nil,
		validator: nil,
		// Cache this early as reflection can be expensive.
		reqIsStructType: reflect.TypeFor[req]().Kind() == reflect.Struct,
	}
}

type jsonHandler[req, res any] struct {
	handler         Handler[req, res]
	logger          *slog.Logger
	validator       *validator.Validate
	reqIsStructType bool
}

// SetLogger is used by the server to inject the logger that will be used by the handler.
func (h *jsonHandler[req, res]) SetLogger(l *slog.Logger) { h.logger = l }

// SetValidator is used by the server to inject the validator that will be used by the handler.
func (h *jsonHandler[req, res]) SetValidator(v *validator.Validate) { h.validator = v }

// ServeHTTP implements the http.Handler interface. It reads the request body,
// decodes it into the request data, validates it if a validator is set,
// calls the wrapped handler, and writes the response back in JSON format.
func (h *jsonHandler[req, res]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	request := Request[req]{Request: r} //nolint:exhaustruct // Zero value of Data is unknown.

	body, err := io.ReadAll(request.Body)
	if err != nil {
		h.logger.WarnContext(r.Context(), "JSON handler failed to read request body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	if !isEmptyStruct(request.Data) {
		if len(body) == 0 {
			h.writeEmptyBodyError(r.Context(), w)
			return
		}

		if err = json.Unmarshal(body, &request.Data); err != nil {
			h.logger.WarnContext(r.Context(), "JSON handler failed to decode request data", slog.String("error", err.Error()))
			// TODO: should we return a reason here or happy with the log?
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		if h.reqIsStructType {
			if err = h.validator.StructCtx(r.Context(), &request.Data); err != nil {
				var invalidValidationError *validator.InvalidValidationError
				if errors.As(err, &invalidValidationError) {
					h.logger.ErrorContext(r.Context(), "JSON handler failed to validate request data", slog.String("error", err.Error()))
					w.WriteHeader(http.StatusInternalServerError)

					return
				}

				var validationErrors validator.ValidationErrors
				if errors.As(err, &validationErrors) {
					h.writeValidationErrors(r.Context(), w, validationErrors)
					return
				}

				h.logger.ErrorContext(r.Context(), "JSON handler received an unknown validation error", slog.String("error", err.Error()))
				w.WriteHeader(http.StatusInternalServerError)

				return
			}
		}

		request.Body = io.NopCloser(bytes.NewBuffer(body))
	}

	response, err := h.handler(request)

	var problemDetails *problem.Details
	if errors.As(err, &problemDetails) {
		w.WriteHeader(problemDetails.Status)
		h.encodeResponse(r.Context(), w, problemDetails)

		return
	}

	var handlerErr *handlerError
	if errors.As(err, &handlerErr) {
		h.writeErrResponse(r.Context(), w, handlerErr)
		return
	}

	if err != nil {
		h.logger.WarnContext(r.Context(), "JSON handler received an unhandled error from inner handler", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	writeHeaders(w, response.Header)
	w.WriteHeader(response.code)

	if _, ok := any(response.data).(struct{}); !ok {
		h.encodeResponse(r.Context(), w, response.data)
	}
}

func (h *jsonHandler[req, res]) encodeResponse(ctx context.Context, w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(&data); err != nil {
		h.logger.ErrorContext(ctx, "JSON handler failed to encode response data", slog.String("error", err.Error()))
	}
}

func (h *jsonHandler[req, res]) writeEmptyBodyError(ctx context.Context, w http.ResponseWriter) {
	noContentErr := struct {
		Error string `json:"error"`
	}{Error: "Empty request body"}

	w.WriteHeader(http.StatusBadRequest)
	h.encodeResponse(ctx, w, noContentErr)
}

func (h *jsonHandler[req, res]) writeErrResponse(ctx context.Context, w http.ResponseWriter, err *handlerError) {
	errResponse := struct {
		Error string `json:"error"`
	}{Error: err.Error()}

	w.WriteHeader(err.code)
	h.encodeResponse(ctx, w, errResponse)
}

func (h *jsonHandler[req, res]) writeValidationErrors(ctx context.Context, w http.ResponseWriter, errs []validator.FieldError) {
	// TODO: problem.ConstraintViolation(r.URL.EscapedPath())
	type validationErr struct {
		Tag   string `json:"tag"`
		Param string `json:"param"`
	}

	validationErrorResponse := struct {
		Error  string                   `json:"error"`
		Errors map[string]validationErr `json:"errors"`
	}{Error: "Invalid request body", Errors: make(map[string]validationErr, len(errs))}

	for _, err := range errs {
		validationErrorResponse.Errors[err.Field()] = validationErr{
			Tag:   err.Tag(),
			Param: err.Param(),
		}
	}

	w.WriteHeader(http.StatusBadRequest)
	h.encodeResponse(ctx, w, validationErrorResponse)
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
