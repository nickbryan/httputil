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

	// Transformer allows for operations to be performed on the Request, Response or
	// Params data before it gets finalized. A Transformer will not be called for a
	// standard http.Handler.
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

type jsonHandler[D, P any] struct {
	action                      Action[D, P]
	guard                       Guard
	logger                      *slog.Logger
	validator                   *validator.Validate
	reqTypeKind, paramsTypeKind reflect.Kind
}

// NewJSONHandler creates a new Handler that wraps the provided [Action] to
// deserialize JSON request bodies and serialize JSON response bodies.
func NewJSONHandler[D, P any](action Action[D, P]) Handler {
	return &jsonHandler[D, P]{
		action:    action,
		guard:     nil,
		logger:    nil,
		validator: nil,
		// Cache these early to save on reflection calls.
		reqTypeKind:    reflect.TypeFor[D]().Kind(),
		paramsTypeKind: reflect.TypeFor[P]().Kind(),
	}
}

func (h *jsonHandler[D, P]) init(l *slog.Logger, v *validator.Validate, g Guard) {
	h.logger, h.validator, h.guard = l, v, g
}

// ServeHTTP implements the http.Handler interface. It reads the request body,
// decodes it into the request data, validates it if a validator is set, calls
// the wrapped Action, and writes the response back in JSON format.
func (h *jsonHandler[D, P]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	//nolint:exhaustruct // Zero value for D and P is unknown.
	if request, ok := h.processRequest(Request[D, P]{Request: r, ResponseWriter: w}); ok {
		response, err := h.action(request)
		if err != nil {
			h.writeErrorResponse(r.Context(), w, fmt.Errorf("calling action: %w", err))
			return
		}

		h.processResponse(request, response)
	}
}

func (h *jsonHandler[D, P]) processRequest(req Request[D, P]) (Request[D, P], bool) {
	if h.guard != nil {
		response, err := h.guard.Guard(req.Request)
		if err != nil {
			h.writeErrorResponse(req.Context(), req.ResponseWriter, fmt.Errorf("calling guard: %w", err))

			return req, false
		}

		if response != nil {
			h.processResponse(req, response)
			return req, false
		}
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to read request body", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return req, false
	}

	if h.paramsTypeKind != reflect.Struct {
		h.logger.WarnContext(req.Context(), "JSON handler params type is not a struct", slog.String("type", h.paramsTypeKind.String()))
	}

	if h.paramsTypeKind == reflect.Struct && !isEmptyStruct(req.Params) {
		if err = UnmarshalParams(req.Request, &req.Params); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to decode params data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.BadRequest(req.Request)) // TODO: need custom errors for param validation.
		}

		if err = h.validator.StructCtx(req.Context(), &req.Params); err != nil {
			h.writeValidationErr(req.ResponseWriter, req.Request, err) // TODO: need custom errors for param validation.
			return req, false
		}

		if err = transform(req.Context(), &req.Params); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to transform params data", slog.Any("error", err))
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

		if h.reqTypeKind == reflect.Struct {
			if err = h.validator.StructCtx(req.Context(), &req.Data); err != nil {
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

	// Put the body contents back so that it can be read in the action again if
	// desired. We have consumed the buffer when reading Body above.
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return req, true
}

func (h *jsonHandler[D, P]) processResponse(req Request[D, P], res *Response) {
	if res == nil {
		return
	}

	if res.redirect != "" {
		http.Redirect(req.ResponseWriter, req.Request, res.redirect, res.code)
		return
	}

	if res.data == nil {
		req.ResponseWriter.WriteHeader(res.code)
		return
	}

	if err := transform(req.Context(), res.data); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to transform response data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return
	}

	req.ResponseWriter.WriteHeader(res.code)
	h.writeResponse(req.Context(), req.ResponseWriter, res.data)
}

func (h *jsonHandler[D, P]) writeValidationErr(w http.ResponseWriter, r *http.Request, err error) {
	var errs validator.ValidationErrors
	if errors.As(err, &errs) {
		properties := make([]problem.Property, 0, len(errs))
		for _, err := range errs {
			properties = append(properties, problem.Property{Detail: explainValidationError(err), Pointer: "#/" + strings.Join(strings.Split(err.Namespace(), ".")[1:], "/")})
		}

		h.writeErrorResponse(r.Context(), w, problem.ConstraintViolation(r, properties...))

		return
	}

	h.logger.ErrorContext(r.Context(), "JSON handler failed to validate request data", slog.Any("error", err))
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

	h.logger.ErrorContext(ctx, "JSON handler received an unhandled error", slog.Any("error", err))
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

func transform(ctx context.Context, data any) error {
	if transformer, ok := data.(Transformer); ok {
		if err := transformer.Transform(ctx); err != nil {
			return fmt.Errorf("transforming data: %w", err)
		}
	}

	return nil
}

type netHTTPHandler struct {
	handler http.Handler
}

func (h netHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

func (h netHTTPHandler) init(_ *slog.Logger, _ *validator.Validate, _ Guard) {}

// NewNetHTTPHandler creates a new Handler that wraps the provided http.Handler
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandler(h http.Handler) Handler {
	return netHTTPHandler{handler: h}
}

// NewNetHTTPHandlerFunc creates a new Handler that wraps the provided http.HandlerFunc
// so that it can be used on an Endpoint definition.
func NewNetHTTPHandlerFunc(h http.HandlerFunc) Handler {
	return netHTTPHandler{handler: h}
}
