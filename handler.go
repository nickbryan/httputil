package httputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"github.com/nickbryan/httputil/problem"
)

type (
	// Action defines the interface for an action function that will be called by
	// the handler. It takes a Request that has data of type D and params of type P
	// and returns a Response or an error.
	Action[D, P any] func(r Request[D, P]) (*Response, error)

	// Guard defines an interface for components that protect access to a Handler's
	// Action. It acts as a crucial pre-processing gatekeeper within the handler's
	// request lifecycle, executing after the request has been routed to the handler,
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

// GuardFunc is a function type for modifying or inspecting an HTTP
// request, potentially returning an altered request. This is useful for
// authentication and adding claims to the request context.
type GuardFunc func(r *http.Request) (*http.Request, error)

// Guard applies the GuardFunc to modify or inspect the provided HTTP request.
func (rif GuardFunc) Guard(r *http.Request) (*http.Request, error) {
	return rif(r)
}

type (
	// Request is a generic HTTP request wrapper that contains request data,
	// parameters, and a response writer.
	Request[D, P any] struct {
		*http.Request
		// Data holds the request-specific data of generic type D, which is provided
		// when initializing the request. A [Handler] will attempt to decode the http.Request
		// body into this type.
		Data D
		// Params holds the parameters of generic type P associated with the request,
		// allowing dynamic decoding and validation of Request parameters. See
		// [BindValidParameters] documentation for usage information.
		Params P
		// ResponseWriter is an embedded HTTP response writer used to construct and send
		// the HTTP response. When writing a response via the ResponseWriter directly, it
		// is best practice to return a [NothingToHandle] response so that the handler
		// does not try to encode response data or handle errors.
		ResponseWriter http.ResponseWriter
	}

	// RequestData represents a Request that expects data but no Params.
	// It's a type alias for Request with a generic data type D and an empty struct for Params.
	// Use this type when your handler needs to process request body data but doesn't need URL parameters.
	RequestData[D any] = Request[D, struct{}]

	// RequestEmpty represents an empty Request that expects no Params or data.
	// It's a type alias for Request with empty structs for both data and Params.
	// Use this type when your handler doesn't need to process any request body or URL parameters.
	RequestEmpty = Request[struct{}, struct{}]

	// RequestParams represents a Request that expects Params but no data.
	// It's a type alias for Request with an empty struct for data and a generic Params type P.
	// Use this type when your handler needs to process URL parameters but doesn't need request body data.
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

// Ensure that our handler implements the http.Handler interface.
var _ http.Handler = &handler[any, any]{} //nolint:exhaustruct // Compile time implementation check.

type handler[D, P any] struct {
	mu                          sync.Mutex
	action                      Action[D, P]
	codec                       ServerCodec
	guard                       Guard
	logger                      *slog.Logger
	reqTypeKind, paramsTypeKind reflect.Kind
}

// NewHandler creates a new Handler that wraps the provided Action. It accepts
// options to configure the handler's behavior.
func NewHandler[D, P any](action Action[D, P], options ...HandlerOption) http.Handler {
	opts := mapHandlerOptionsToDefaults(options)

	return &handler[D, P]{
		mu:     sync.Mutex{},
		action: action,
		// Cache these early to save on reflection calls.
		reqTypeKind:    reflect.TypeFor[D]().Kind(),
		paramsTypeKind: reflect.TypeFor[P]().Kind(),
		// The server will inject these if they are not set by an option.
		codec:  opts.codec,
		guard:  opts.guard,
		logger: opts.logger,
	}
}

// setCodec sets the codec for the handler if it has not already been set.
func (h *handler[D, P]) setCodec(c ServerCodec) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.codec == nil {
		h.codec = c
	}
}

// setGuard sets the guard for the handler if it has not already been set.
func (h *handler[D, P]) setGuard(g Guard) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.guard == nil {
		h.guard = g
	}
}

// setLogger sets the logger for the handler if it has not already been set.
func (h *handler[D, P]) setLogger(l *slog.Logger) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.logger == nil {
		h.logger = l
	}
}

// ServeHTTP implements the http.Handler interface. It reads the request body,
// decodes it into the request data, validates it if a validator is set, calls
// the wrapped Action, and writes the response back in JSON format.
func (h *handler[D, P]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer closeRequestBody(r.Context(), h.logger, r.Body)

	//nolint:exhaustruct // Zero value for D and P is unknown.
	request := Request[D, P]{Request: r, ResponseWriter: w}

	if err := h.protect(&request); err != nil {
		h.writeErrorResponse(r.Context(), &request, err)
		return
	}

	if !h.requestHydratedOK(&request) {
		return
	}

	response, err := h.action(request)
	if err != nil {
		h.writeErrorResponse(r.Context(), &request, fmt.Errorf("calling action: %w", err))
		return
	}

	h.writeSuccessfulResponse(&request, response)
}

// protect runs the guard if one is set which may modify the request
// or block further processing if an error is returned.
func (h *handler[D, P]) protect(req *Request[D, P]) error {
	if h.guard == nil {
		return nil
	}

	protectedRequest, err := h.guard.Guard(req.Request)
	if err != nil {
		return fmt.Errorf("calling guard: %w", err)
	}

	if protectedRequest != nil {
		req.Request = protectedRequest
	}

	return nil
}

// requestHydratedOK validates and processes the request payload and parameters,
// ensuring the request is properly hydrated.
func (h *handler[D, P]) requestHydratedOK(req *Request[D, P]) bool {
	if !h.paramsHydratedOK(req) {
		return false
	}

	if isEmpty(req.Data) {
		return true
	}

	if err := h.codec.Decode(req.Request, &req.Data); err != nil {
		problemErr := problem.BadRequest(req.Request)

		if errors.Is(err, io.EOF) {
			problemErr = problem.BadRequest(req.Request).WithDetail("The server received an unexpected empty request body")
		} else {
			h.logger.WarnContext(req.Context(), "Handler failed to decode request data", slog.Any("error", err))
		}

		h.writeErrorResponse(req.Context(), req, problemErr)

		return false
	}

	if h.reqTypeKind == reflect.Struct {
		if err := validate.StructCtx(req.Context(), &req.Data); err != nil {
			h.writeValidationErr(req, err)
			return false
		}
	}

	if err := transform(req.Context(), &req.Data); err != nil {
		h.logger.WarnContext(req.Context(), "Handler failed to transform request data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return false
	}

	return true
}

// paramsHydratedOK checks if the request parameters are valid, hydrated, and
// successfully transformed without errors.
func (h *handler[D, P]) paramsHydratedOK(req *Request[D, P]) bool {
	if isEmpty(req.Params) {
		return true
	}

	if h.paramsTypeKind != reflect.Struct {
		h.logger.WarnContext(req.Context(), "Handler params type is not a struct", slog.String("type", h.paramsTypeKind.String()))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return false
	}

	if err := BindValidParameters(req.Request, &req.Params); err != nil {
		var detailedError *problem.DetailedError
		if !errors.As(err, &detailedError) {
			h.logger.WarnContext(req.Context(), "Handler failed to decode params data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

			return false
		}

		h.writeErrorResponse(req.Context(), req, err)

		return false
	}

	if err := transform(req.Context(), &req.Params); err != nil {
		h.logger.WarnContext(req.Context(), "Handler failed to transform params data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return false
	}

	return true
}

// writeSuccessfulResponse writes a successful HTTP response to the client,
// handling redirects, empty data, or JSON encoding.
func (h *handler[D, P]) writeSuccessfulResponse(req *Request[D, P], res *Response) {
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
		h.logger.WarnContext(req.Context(), "Handler failed to transform response data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return
	}

	if err := h.codec.Encode(req.ResponseWriter, res.code, res.data); err != nil {
		h.logger.ErrorContext(req.Context(), "Handler failed to encode response data", slog.Any("error", err))
	}
}

// writeValidationErr handles validation errors by constructing detailed problem
// objects and writing error responses. If the error is not a validation error,
// it logs the error and sends a generic server error response.
func (h *handler[D, P]) writeValidationErr(req *Request[D, P], err error) {
	var errs validator.ValidationErrors
	if errors.As(err, &errs) {
		properties := make([]problem.Property, 0, len(errs))
		for _, err := range errs {
			properties = append(properties, problem.Property{Detail: describeValidationError(err), Pointer: "/" + strings.Join(strings.Split(err.Namespace(), ".")[1:], "/")})
		}

		h.writeErrorResponse(req.Context(), req, problem.ConstraintViolation(req.Request, properties...))

		return
	}

	h.logger.ErrorContext(req.Context(), "Handler failed to validate request data", slog.Any("error", err))
	h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))
}

// writeErrorResponse writes an HTTP error response using the provided error and
// request context, with support for problem details.
func (h *handler[D, P]) writeErrorResponse(ctx context.Context, req *Request[D, P], err error) {
	var problemDetails *problem.DetailedError
	if !errors.As(err, &problemDetails) {
		problemDetails = problem.ServerError(req.Request)

		h.logger.ErrorContext(ctx, "Handler received an unhandled error", slog.Any("error", err))
	}

	if err = h.codec.EncodeError(req.ResponseWriter, problemDetails.Status, problemDetails); err != nil {
		h.logger.ErrorContext(ctx, "Handler failed to encode error data", slog.Any("error", err))
	}
}

// transform applies a transformation to the given data if it implements the
// Transformer interface. It returns an error if the transformation fails;
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

// closeRequestBody safely closes the request body and logs a warning if an
// error occurs during closure.
func closeRequestBody(ctx context.Context, logger *slog.Logger, body io.Closer) {
	if body == nil {
		return
	}

	if err := body.Close(); err != nil {
		logger.WarnContext(ctx, "Handler failed to close request body", slog.Any("error", err))
	}
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}

	_, ok := v.(struct{})

	return ok
}
