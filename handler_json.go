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

// Ensure that our jsonHandler implements the Handler interface.
var _ Handler = &jsonHandler[any, any]{} //nolint:exhaustruct // Compile time implementation check.

// jsonHandler is a generic struct for handling HTTP requests and responses with
// JSON encoding and decoding. It supports custom actions, logging, and request
// interception. D and P represent the data and parameter types processed by the
// handler, respectively.
type jsonHandler[D, P any] struct {
	action                      Action[D, P]
	requestInterceptor          RequestInterceptor
	logger                      *slog.Logger
	reqTypeKind, paramsTypeKind reflect.Kind
}

// NewJSONHandler creates a new Handler that wraps the provided [Action] to
// deserialize JSON request bodies and serialize JSON response bodies.
func NewJSONHandler[D, P any](action Action[D, P]) Handler {
	return &jsonHandler[D, P]{
		action:             action,
		requestInterceptor: nil,
		logger:             nil,
		// Cache these early to save on reflection calls.
		reqTypeKind:    reflect.TypeFor[D]().Kind(),
		paramsTypeKind: reflect.TypeFor[P]().Kind(),
	}
}

// use sets the logger and request interceptor for the JSON handler to allow
// dependencies to be injected from the Server.
func (h *jsonHandler[D, P]) use(l *slog.Logger, g RequestInterceptor) {
	h.logger, h.requestInterceptor = l, g
}

// ServeHTTP implements the http.Handler interface. It reads the request body,
// decodes it into the request data, validates it if a validator is set, calls
// the wrapped Action, and writes the response back in JSON format.
func (h *jsonHandler[D, P]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	//nolint:exhaustruct // Zero value for D and P is unknown.
	request := Request[D, P]{Request: r, ResponseWriter: w}

	if h.interceptBlocksHandler(&request) || !h.requestHydratedOK(&request) {
		return
	}

	response, err := h.action(request)
	if err != nil {
		h.writeErrorResponse(r.Context(), &request, fmt.Errorf("calling action: %w", err))
		return
	}

	h.writeSuccessfulResponse(&request, response)
}

// interceptBlocksHandler handles request interception, modifying the request or
// blocking further processing if needed.
func (h *jsonHandler[D, P]) interceptBlocksHandler(req *Request[D, P]) bool {
	if h.requestInterceptor == nil {
		return false
	}

	interceptedRequest, err := h.requestInterceptor.InterceptRequest(req.Request)
	if err != nil {
		h.writeErrorResponse(req.Context(), req, fmt.Errorf("calling request interceptor: %w", err))
		return true
	}

	if interceptedRequest != nil {
		req.Request = interceptedRequest
	}

	return false
}

// requestHydratedOK validates and processes the request payload and parameters,
// ensuring the request is properly hydrated.
func (h *jsonHandler[D, P]) requestHydratedOK(req *Request[D, P]) bool {
	if !h.paramsHydratedOK(req) {
		return false
	}

	if req.Body == nil {
		return true
	}

	defer func(body io.Closer) {
		if err := body.Close(); err != nil {
			h.logger.WarnContext(req.Context(), "JSON handler failed to close request body", slog.Any("error", err))
		}
	}(req.Body)

	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to read request body", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return false
	}

	if !h.dataHydratedOK(req, body) {
		return false
	}

	// Put the body contents back so that it can be read in the action again if
	// desired. We have consumed the buffer when reading Body above.
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return true
}

// paramsHydratedOK checks if the request parameters are valid, hydrated, and
// successfully transformed without errors.
func (h *jsonHandler[D, P]) paramsHydratedOK(req *Request[D, P]) bool {
	if h.paramsTypeKind != reflect.Struct {
		h.logger.WarnContext(req.Context(), "JSON handler params type is not a struct", slog.String("type", h.paramsTypeKind.String()))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return false
	}

	if isEmptyStruct(req.Params) {
		return true
	}

	if err := BindValidParameters(req.Request, &req.Params); err != nil {
		var detailedError *problem.DetailedError
		if !errors.As(err, &detailedError) {
			h.logger.WarnContext(req.Context(), "JSON handler failed to decode params data", slog.Any("error", err))
			h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

			return false
		}

		h.writeErrorResponse(req.Context(), req, err)

		return false
	}

	if err := transform(req.Context(), &req.Params); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to transform params data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return false
	}

	return true
}

// dataHydratedOK checks if the request data is successfully hydrated and
// validates it against the expected structure and transformations.
func (h *jsonHandler[D, P]) dataHydratedOK(req *Request[D, P], body []byte) bool {
	if isEmptyStruct(req.Data) {
		return true
	}

	if len(body) == 0 {
		h.writeErrorResponse(req.Context(), req, problem.BadRequest(req.Request).WithDetail("The server received an unexpected empty request body"))
		return false
	}

	if err := json.Unmarshal(body, &req.Data); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to decode request data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req, problem.BadRequest(req.Request))

		return false
	}

	if h.reqTypeKind == reflect.Struct {
		if err := validate.StructCtx(req.Context(), &req.Data); err != nil {
			h.writeValidationErr(req, err)
			return false
		}
	}

	if err := transform(req.Context(), &req.Data); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to transform request data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return false
	}

	return true
}

// writeSuccessfulResponse writes a successful HTTP response to the client,
// handling redirects, empty data, or JSON encoding.
func (h *jsonHandler[D, P]) writeSuccessfulResponse(req *Request[D, P], res *Response) {
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
		h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))

		return
	}

	req.ResponseWriter.WriteHeader(res.code)
	h.writeResponse(req.Context(), req.ResponseWriter, res.data)
}

// writeValidationErr handles validation errors by constructing detailed problem
// objects and writing error responses. If the error is not a validation error,
// it logs the error and sends a generic server error response.
func (h *jsonHandler[D, P]) writeValidationErr(req *Request[D, P], err error) {
	var errs validator.ValidationErrors
	if errors.As(err, &errs) {
		properties := make([]problem.Property, 0, len(errs))
		for _, err := range errs {
			properties = append(properties, problem.Property{Detail: describeValidationError(err), Pointer: "/" + strings.Join(strings.Split(err.Namespace(), ".")[1:], "/")})
		}

		h.writeErrorResponse(req.Context(), req, problem.ConstraintViolation(req.Request, properties...))

		return
	}

	h.logger.ErrorContext(req.Context(), "JSON handler failed to validate request data", slog.Any("error", err))
	h.writeErrorResponse(req.Context(), req, problem.ServerError(req.Request))
}

// writeErrorResponse writes an HTTP error response using the provided error and
// request context, with support for problem details.
func (h *jsonHandler[D, P]) writeErrorResponse(ctx context.Context, req *Request[D, P], err error) {
	req.ResponseWriter.Header().Set("Content-Type", "application/problem+json")

	var problemDetails *problem.DetailedError
	if !errors.As(err, &problemDetails) {
		problemDetails = problem.ServerError(req.Request)

		h.logger.ErrorContext(ctx, "JSON handler received an unhandled error", slog.Any("error", err))
	}

	req.ResponseWriter.WriteHeader(problemDetails.Status)
	h.writeResponse(ctx, req.ResponseWriter, problemDetails)
}

// writeResponse encodes the provided data into JSON and writes it to the HTTP
// response writer and logs errors if encoding fails.
func (h *jsonHandler[D, P]) writeResponse(ctx context.Context, w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(&data); err != nil {
		h.logger.ErrorContext(ctx, "JSON handler failed to encode response data", slog.Any("error", err))
	}
}
