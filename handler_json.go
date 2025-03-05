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
	request := Request[D, P]{Request: r, ResponseWriter: w}

	if h.requestInterceptedByGuard(request) || !h.requestHydratedOK(&request) {
		return
	}

	response, err := h.action(request)
	if err != nil {
		h.writeErrorResponse(r.Context(), w, fmt.Errorf("calling action: %w", err))
		return
	}

	h.writeSuccessfulResponse(request, response) //nolint:contextcheck // Context is part of the request struct.
}

func (h *jsonHandler[D, P]) requestInterceptedByGuard(req Request[D, P]) bool {
	if h.guard == nil {
		return false
	}

	response, err := h.guard.Guard(req.Request)
	if err != nil {
		h.writeErrorResponse(req.Context(), req.ResponseWriter, fmt.Errorf("calling guard: %w", err))
		return true
	}

	if response != nil {
		h.writeSuccessfulResponse(req, response)
		return true
	}

	return false
}

func (h *jsonHandler[D, P]) requestHydratedOK(req *Request[D, P]) bool {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to read request body", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return false
	}

	if !h.paramsHydratedOK(req) || !h.dataHydratedOK(req, body) {
		return false
	}

	// Put the body contents back so that it can be read in the action again if
	// desired. We have consumed the buffer when reading Body above.
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return true
}

func (h *jsonHandler[D, P]) paramsHydratedOK(req *Request[D, P]) bool {
	if h.paramsTypeKind != reflect.Struct {
		h.logger.WarnContext(req.Context(), "JSON handler params type is not a struct", slog.String("type", h.paramsTypeKind.String()))
		return false
	}

	if isEmptyStruct(req.Params) {
		return true
	}

	if err := UnmarshalParams(req.Request, &req.Params); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to decode params data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.BadRequest(req.Request)) // TODO: need custom errors for param validation.

		return false
	}

	if err := h.validator.StructCtx(req.Context(), &req.Params); err != nil {
		h.writeValidationErr(req.ResponseWriter, req.Request, err) // TODO: need custom errors for param validation.
		return false
	}

	if err := transform(req.Context(), &req.Params); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to transform params data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return false
	}

	return true
}

func (h *jsonHandler[D, P]) dataHydratedOK(req *Request[D, P], body []byte) bool {
	if isEmptyStruct(req.Data) {
		return true
	}

	if len(body) == 0 {
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.BadRequest(req.Request).WithDetail("The server received an unexpected empty request body"))
		return false
	}

	if err := json.Unmarshal(body, &req.Data); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to decode request data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.BadRequest(req.Request))

		return false
	}

	if h.reqTypeKind == reflect.Struct {
		if err := h.validator.StructCtx(req.Context(), &req.Data); err != nil {
			h.writeValidationErr(req.ResponseWriter, req.Request, err)
			return false
		}
	}

	if err := transform(req.Context(), &req.Data); err != nil {
		h.logger.WarnContext(req.Context(), "JSON handler failed to transform request data", slog.Any("error", err))
		h.writeErrorResponse(req.Context(), req.ResponseWriter, problem.ServerError(req.Request))

		return false
	}

	return true
}

func (h *jsonHandler[D, P]) writeSuccessfulResponse(req Request[D, P], res *Response) {
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
