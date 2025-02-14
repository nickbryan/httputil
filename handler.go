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
	Handler[D any] func(r Request[D]) (*Response, error)

	// Request represents an HTTP request with additional data of type `T`.
	Request[D any] struct {
		*http.Request
		Data           D
		ResponseWriter http.ResponseWriter
	}

	// RequestNoBody represents an empty Request.
	RequestNoBody = Request[struct{}]

	// Response represents an HTTP response that holds optional data and the
	// required information to write a response.
	Response struct {
		code     int
		data     any
		redirect string
	}

	// Transformer allows for transforms to be performed on the
	// Request or Response data before it gets finalized.
	Transformer interface {
		Transform(ctx context.Context) error
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

// NewResponseAccepted creates a new Response object with a status code of
// http.StatusAccepted (202 Accepted) and an empty struct as data.
func NewResponseAccepted() *Response {
	return &Response{
		code:     http.StatusAccepted,
		data:     nil,
		redirect: "",
	}
}

// NewResponseCreated creates a new Response object with a status code of
// http.StatusCreated (201 Created) and an empty struct as data.
func NewResponseCreated() *Response {
	return &Response{
		code:     http.StatusCreated,
		data:     nil,
		redirect: "",
	}
}

// NewResponseNoContent creates a new Response object with a status code of
// http.StatusNoContent (204 No Content) and an empty struct as data.
func NewResponseNoContent() *Response {
	return &Response{
		code:     http.StatusNoContent,
		data:     nil,
		redirect: "",
	}
}

// NewResponseRedirect creates a new Response object with the given status code
// and an empty struct as data. The redirect url will be set which will
// indicate to the Handler that a redirect should be written.
func NewResponseRedirect(code int, url string) *Response {
	return &Response{
		code:     code,
		data:     nil,
		redirect: url,
	}
}

type jsonHandler[D any] struct {
	handler         Handler[D]
	logger          *slog.Logger
	reqIsStructType bool
}

// NewJSONHandler creates a new http.Handler that wraps the provided [Handler]
// function to deserialize JSON request bodies and serialize JSON response
// bodies.
func NewJSONHandler[D any](handler Handler[D]) http.Handler {
	return &jsonHandler[D]{
		handler: handler,
		logger:  nil,
		// Cache this early as reflection can be expensive.
		reqIsStructType: reflect.TypeFor[D]().Kind() == reflect.Struct,
	}
}

// setLogger is used by the server to inject the logger that will be used by the handler.
func (h *jsonHandler[D]) setLogger(l *slog.Logger) { h.logger = l }

// ServeHTTP implements the http.Handler interface. It reads the request body,
// decodes it into the request data, validates it if a validator is set, calls
// the wrapped handler, and writes the response back in JSON format.
func (h *jsonHandler[D]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	//nolint:exhaustruct // Zero value of Data is unknown.
	request, ok := h.processRequest(Request[D]{Request: r, ResponseWriter: w})
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

func (h *jsonHandler[D]) processRequest(req Request[D]) (Request[D], bool) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to read request body", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return req, false
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

		if err = transform(req.Context(), &req.Data); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to transform request data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

			return req, false
		}
	}

	// Put the body contents back so that it can be read in the handler again if
	// desired. We have consumed the buffer when reading Body above.
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return req, true
}

func (h *jsonHandler[D]) processResponse(req Request[D], res *Response) {
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

	if err := transform(req.Context(), &res.data); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to transform res data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return
	}

	h.writeResponse(req.Context(), req.ResponseWriter, res.data)
}

func explainValidationError(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return err.Field() + " is required"
	case "email":
		return err.Field() + " should be a valid email"
	case "uuid":
		return err.Field() + " should be a valid UUID"
	case "e164":
		return err.Field() + " should be a valid international phone number (e.g. +33 6 06 06 06 06)"
	default:
		resp := fmt.Sprintf("%s should be %s", err.Field(), err.Tag())
		if err.Param() != "" {
			resp += "=" + err.Param()
		}

		return resp
	}
}

func (h *jsonHandler[D]) writeValidationErr(w http.ResponseWriter, r *http.Request, err error) {
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

func (h *jsonHandler[D]) writeErrorResponse(ctx context.Context, w http.ResponseWriter, err error) {
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

func (h *jsonHandler[D]) writeResponse(ctx context.Context, w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(&data); err != nil {
		h.logger.ErrorContext(ctx, "JSON handler failed to encode response data", slog.Any("error", err))
	}
}

func isEmptyStruct(v any) bool {
	_, ok := v.(struct{})
	return ok
}

func transform(ctx context.Context, data any) error {
	if transformer, ok := data.(Transformer); ok {
		if err := transformer.Transform(ctx); err != nil {
			return fmt.Errorf("transforming data: %w", err)
		}
	}

	return nil
}
