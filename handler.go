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
)

type (
	Handler[Req, Res any] func(r Request[Req]) (*Response[Res], error)

	Request[T any] struct {
		*http.Request
		Data T
	}

	Response[T any] struct {
		Header     http.Header
		data       T
		statusCode int
	}
)

func NewResponse[T any](statusCode int, data T) *Response[T] {
	return &Response[T]{
		Header:     make(http.Header),
		data:       data,
		statusCode: statusCode,
	}
}

// TODO: Decide if this is needed and update tests to use it if so.
// TODO: statusCode to status?
// TODO: drop status code in favour of StatusNoContent
func NewEmptyResponse(statusCode int) *Response[struct{}] {
	return &Response[struct{}]{
		Header:     make(http.Header),
		data:       struct{}{},
		statusCode: statusCode,
	}
}

// TODO: export type and drop new function?
type handlerError struct {
	message    string
	statusCode int
}

func (e *handlerError) Error() string {
	return e.message
}

// ErrInternal is used when the error is unknown to the caller.
var ErrInternal = &handlerError{
	message:    "internal server error",
	statusCode: http.StatusInternalServerError,
}

// NewHandlerError will create a new error response that will have the given
// message as the error field and set the supplied status code.
func NewHandlerError(statusCode int, message string) error {
	return &handlerError{
		message:    message,
		statusCode: statusCode,
	}
}

func NewJSONHandler[Req, Res any](handler Handler[Req, Res]) http.Handler {
	return &jsonHandler[Req, Res]{handler: handler, logger: nil}
}

type jsonHandler[Req, Res any] struct {
	handler   Handler[Req, Res]
	logger    *slog.Logger
	validator *validator.Validate
}

// SetLogger is used by the server to inject the logger that will be used by the handler.
func (h *jsonHandler[Req, Res]) SetLogger(l *slog.Logger) { h.logger = l }

// SetValidator is used by the server to inject the validator that will be used by the handler.
func (h *jsonHandler[Req, Res]) SetValidator(v *validator.Validate) { h.validator = v }

func (h *jsonHandler[Req, Res]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	request := Request[Req]{Request: r}

	body, err := io.ReadAll(request.Body)
	if err != nil {
		h.logger.WarnContext(r.Context(), "JSON handler failed to read request body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	if _, ok := any(request.Data).(struct{}); !ok {
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

		// TODO: Can we cache this reflection call or maybe other calls as we only need to know the type once in order to know to validate or not?
		// TODO: Can this be reflect.TypeFor? It looks like it would work. What happens if a pointer to struct is passed here?
		if reflect.ValueOf(request.Data).Kind() == reflect.Struct {
			if err = h.validator.Struct(&request.Data); err != nil {
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

	var errResponse *handlerError

	response, err := h.handler(request)
	if errors.As(err, &errResponse) {
		h.writeErrResponse(r.Context(), w, errResponse)
		return
	}

	if err != nil {
		h.logger.WarnContext(r.Context(), "JSON handler received an unhandled error from inner handler", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	writeHeaders(w, response.Header)
	w.WriteHeader(response.statusCode)

	if _, ok := any(response.data).(struct{}); !ok {
		if err := json.NewEncoder(w).Encode(response.data); err != nil {
			h.logger.ErrorContext(r.Context(), "JSON handler failed to encode response data", slog.String("error", err.Error()))
		}
	}
}

func writeHeaders(w http.ResponseWriter, headers http.Header) {
	for header, values := range headers {
		for _, value := range values {
			w.Header().Add(header, value)
		}
	}
}

func (h *jsonHandler[req, res]) writeEmptyBodyError(ctx context.Context, responseWriter http.ResponseWriter) {
	noContentErr := struct {
		Error string `json:"error"`
	}{
		Error: "request body is empty",
	}

	responseWriter.WriteHeader(http.StatusBadRequest)

	if err := json.NewEncoder(responseWriter).Encode(&noContentErr); err != nil {
		h.logger.ErrorContext(ctx, "unable to encode response as json", slog.String("error", err.Error()))
	}
}

func (h *jsonHandler[req, res]) writeErrResponse(ctx context.Context, w http.ResponseWriter, err *handlerError) {
	errResponse := struct {
		Error string `json:"error"`
	}{
		Error: err.Error(),
	}

	w.WriteHeader(err.statusCode)

	if err := json.NewEncoder(w).Encode(&errResponse); err != nil {
		h.logger.ErrorContext(ctx, "unable to encode response as json", slog.String("error", err.Error()))
	}
}

func (h *jsonHandler[req, res]) writeValidationErrors(ctx context.Context, w http.ResponseWriter, errs []validator.FieldError) {
	type validationErr struct {
		Tag   string `json:"tag"`
		Param string `json:"param"`
	}

	validationErrorResponse := struct {
		Error  string                   `json:"error"`
		Errors map[string]validationErr `json:"errors"`
	}{
		Error:  "request validation failed",
		Errors: make(map[string]validationErr, len(errs)),
	}

	for _, err := range errs {
		validationErrorResponse.Errors[err.Field()] = validationErr{
			Tag:   err.Tag(),
			Param: err.Param(),
		}
	}

	w.WriteHeader(http.StatusBadRequest)

	if err := json.NewEncoder(w).Encode(&validationErrorResponse); err != nil {
		h.logger.ErrorContext(ctx, "unable to encode response as json", slog.String("error", err.Error()))
	}
}
