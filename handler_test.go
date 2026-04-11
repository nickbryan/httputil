package httputil_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/internal/testutil"
	"github.com/nickbryan/httputil/problem"
)

func TestNewHandler(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		request                *http.Request
		endpoint               httputil.Endpoint
		wantLogs               []slogmem.RecordQuery
		wantHeader             http.Header
		wantResponseBody       string
		wantResponseStatusCode int
	}{
		"the response content type is not set when a successful response without data is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			wantHeader:             http.Header{},
			wantResponseStatusCode: http.StatusNoContent,
		},
		"the response content type is application/json when a successful response with data is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"hello": "world"})
				}),
			},
			wantHeader:             http.Header{"Content-Type": {"application/json; charset=utf-8"}},
			wantResponseBody:       `{"hello":"world"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"the response content type is application/problem+json when an error response is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, errors.New("some error")
				}),
			},
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling action: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"the response content type is application/problem+json when a problem response is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, problem.ServerError(r.Request)
				}),
			},
			wantHeader:             http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns an internal server error status code and logs a warning when the request body cannot be read": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestData[map[string]any]) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", errReader("the request body was invalid")),
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to decode request data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("decoding request body as JSON: the request body was invalid"),
				},
			}},
			wantResponseBody:       problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns a bad request status code with errors if the payload is empty but request data is expected": {
			endpoint: func() httputil.Endpoint {
				type request struct {
					Name string `json:"name"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(_ httputil.RequestData[request]) (*httputil.Response, error) {
						return httputil.NoContent()
					}),
				}
			}(),
			request:                httptest.NewRequest(http.MethodGet, "/test", strings.NewReader("")),
			wantHeader:             http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantResponseBody:       problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).WithDetail("The server received an unexpected empty request body").MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns a bad request status code and logs a warning when the request body cannot be decoded as json": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestData[map[string]string]) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			request:    httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{`)),
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to decode request data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("decoding request body as JSON: unexpected EOF"),
				},
			}},
			wantResponseBody:       problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns an unprocessable entity request status code with errors if the payload fails validation": {
			endpoint: func() httputil.Endpoint {
				type inner struct {
					Thing string `json:"thing" validate:"required"`
				}

				type request struct {
					Name  string `json:"name"`
					Inner inner  `json:"inner"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(_ httputil.RequestData[request]) (*httputil.Response, error) {
						return httputil.NoContent()
					}),
				}
			}(),
			request:    httptest.NewRequest(http.MethodGet, "/test", strings.NewReader("{}")),
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantResponseBody: problem.ConstraintViolation(
				httptest.NewRequest(http.MethodGet, "/test", http.NoBody),
				problem.Property{Detail: "is required", Pointer: "/inner/thing"},
			).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusUnprocessableEntity,
		},
		"describes the validation errors appropriately": {
			endpoint: func() httputil.Endpoint {
				type request struct {
					Required string `json:"required" validate:"required"`
					Email    string `json:"email"    validate:"email"`
					UUID     string `json:"uuid"     validate:"uuid"`
					UUID4    string `json:"uuid4"    validate:"uuid4"`
					Phone    string `json:"phone"    validate:"e164"`
					Field    string `json:"field"    validate:"min=3"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(_ httputil.RequestData[request]) (*httputil.Response, error) {
						return httputil.NoContent()
					}),
				}
			}(),
			request:    httptest.NewRequest(http.MethodGet, "/test", strings.NewReader("{}")),
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantResponseBody: problem.ConstraintViolation(
				httptest.NewRequest(http.MethodGet, "/test", http.NoBody),
				problem.Property{Detail: "is required", Pointer: "/required"},
				problem.Property{Detail: "should be a valid email", Pointer: "/email"},
				problem.Property{Detail: "should be a valid UUID", Pointer: "/uuid"},
				problem.Property{Detail: "should be a valid UUID4", Pointer: "/uuid4"},
				problem.Property{Detail: "should be a valid international phone number (e.g. +33 6 06 06 06 06)", Pointer: "/phone"},
				problem.Property{Detail: "should be min=3", Pointer: "/field"},
			).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusUnprocessableEntity,
		},
		"custom MessageFunc overrides validation error messages in constraint violation response": {
			endpoint: func() httputil.Endpoint {
				type request struct {
					Name  string `json:"name"  validate:"required"`
					Email string `json:"email" validate:"required,email"`
				}

				return httputil.Endpoint{
					Method: http.MethodPost,
					Path:   "/test",
					Handler: httputil.NewHandler(
						func(_ httputil.RequestData[request]) (*httputil.Response, error) {
							return httputil.NoContent()
						},
						httputil.WithHandlerMessages(func(tag, _ string) string {
							switch tag {
							case "required":
								return "ce champ est obligatoire"
							default:
								return "valeur invalide"
							}
						}),
					),
				}
			}(),
			request:    httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}")),
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantResponseBody: problem.ConstraintViolation(
				httptest.NewRequest(http.MethodGet, "/test", http.NoBody),
				problem.Property{Detail: "ce champ est obligatoire", Pointer: "/name"},
				problem.Property{Detail: "ce champ est obligatoire", Pointer: "/email"},
			).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusUnprocessableEntity,
		},
		"custom MessageFunc receives tag and param for parameterised validators": {
			endpoint: func() httputil.Endpoint {
				type request struct {
					Name string `json:"name" validate:"required,min=3"`
				}

				return httputil.Endpoint{
					Method: http.MethodPost,
					Path:   "/test",
					Handler: httputil.NewHandler(
						func(_ httputil.RequestData[request]) (*httputil.Response, error) {
							return httputil.NoContent()
						},
						httputil.WithHandlerMessages(func(tag, param string) string {
							if tag == "min" {
								return "doit contenir au moins " + param + " caractères"
							}

							return "valeur invalide"
						}),
					),
				}
			}(),
			request:    httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"name":"ab"}`)),
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantResponseBody: problem.ConstraintViolation(
				httptest.NewRequest(http.MethodGet, "/test", http.NoBody),
				problem.Property{Detail: "doit contenir au moins 3 caractères", Pointer: "/name"},
			).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusUnprocessableEntity,
		},
		"the request body is mapped to the requests data": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(r httputil.RequestData[map[string]string]) (*httputil.Response, error) {
					if r.Data["hello"] != "world" {
						t.Errorf("r.data[\"hello\"] = %v, want: world", r.Data["hello"])
					}

					return httputil.NoContent()
				}),
			},
			request:                httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{"hello":"world"}`)),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"an internal server error is returned and a log is written when a generic error is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, errors.New("some error")
				}),
			},
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling action: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"status code is used from the response on successful request": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Accepted(nil)
				}),
			},
			wantResponseStatusCode: http.StatusAccepted,
		},
		"response data is encoded as json in the body": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"hello": "world"})
				}),
			},
			wantResponseBody:       `{"hello":"world"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"logs a warning when the response body cannot be encoded as json": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Created(map[string]chan int{"chan": make(chan int)})
				}),
			},
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to encode response data",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("encoding response data as JSON: json: unsupported type: chan int"),
				},
			}},
			// We have no way to overwrite the status code to an error code in this situation as it will
			// have already been written.
			wantResponseStatusCode: http.StatusCreated,
		},
		"only handles the error case when both an error and a response is returned from the action": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NewResponse(http.StatusNoContent, nil), errors.New("some error")
				}),
			},
			wantHeader: http.Header{"Content-Type": {"application/problem+json; charset=utf-8"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling action: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"redirects the request when a redirect response is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Redirect(http.StatusPermanentRedirect, "http://example.com")
				}),
			},
			wantHeader:             http.Header{"Content-Type": []string{"text/html; charset=utf-8"}, "Location": []string{"http://example.com"}},
			wantResponseBody:       `<a href="http://example.com">Permanent Redirect</a>.`,
			wantResponseStatusCode: http.StatusPermanentRedirect,
		},
		"allows writing to the response writer directly": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					r.ResponseWriter.Header().Set("X-Correlation-Id", "some-random-id")
					r.ResponseWriter.WriteHeader(http.StatusTeapot)

					return httputil.NothingToHandle()
				}),
			},
			wantHeader:             http.Header{"X-Correlation-Id": []string{"some-random-id"}},
			wantResponseStatusCode: http.StatusTeapot,
		},
		"request data is transformed before the action is called": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(r httputil.RequestData[dataFromCtxTransformer]) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"data": r.Data.TransformedData})
				}),
			},
			request: httptest.NewRequestWithContext(
				context.WithValue(t.Context(), ctxKeyData{}, "overridden-data"),
				http.MethodGet,
				"/test",
				strings.NewReader(`{"data":"some-data"}`),
			),
			wantResponseBody:       `{"data":"overridden-data"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"returns and logs an error when the request data transformer returns an error": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestData[errorTransformer]) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"data": "should not be returned"})
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{"data":"some-data"}`)),
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to transform request data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("transforming data: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"params data is transformed before the action is called": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(r httputil.RequestParams[dataFromCtxTransformer]) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"data": r.Params.TransformedData})
				}),
			},
			request: func() *http.Request {
				req := httptest.NewRequestWithContext(
					context.WithValue(t.Context(), ctxKeyData{}, "overridden-data"),
					http.MethodGet,
					"/test",
					nil,
				)
				req.Header.Set("X-Data", "some-data")

				return req
			}(),
			wantResponseBody:       `{"data":"overridden-data"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"returns and logs an error when the params data transformer returns an error": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestParams[errorTransformer]) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"data": "should not be returned"})
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{"data":"some-data"}`)),
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to transform params data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("transforming data: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"response data is transformed after the action is called": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.OK(&dataFromCtxTransformer{TransformedData: "some-data"})
				}),
			},
			request: httptest.NewRequestWithContext(
				context.WithValue(t.Context(), ctxKeyData{}, "overridden-data"),
				http.MethodGet,
				"/test",
				nil,
			),
			wantResponseBody:       `{"data":"overridden-data"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"returns and logs an error when the response data transformer returns an error": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.OK(errorTransformer{})
				}),
			},
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to transform response data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("transforming data: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns the response when a guard is set as nil": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, nil),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"returns the response when the guard does not block the handler": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, noopGuard{}),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"returns and logs an error when the guard blocks the handler by returning an error": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, errorGuard{}),
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling guard: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns a problem error when the guard blocks the handler by returning a problem error type": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, problemGuard{}),
			wantResponseBody:       problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"allows the guard to add to the request context which is passed to the handler for consumption": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					ctxVal, ok := r.Context().Value(addToContextGuardCtxKey{}).(addToContextGuard)
					if !ok {
						return nil, problem.BusinessRuleViolation(r.Request).WithDetail("ctxVal not set")
					}

					return httputil.OK(map[string]string{"context": string(ctxVal)})
				}),
			}, addToContextGuard("my context value")),
			request: httptest.NewRequestWithContext(
				context.WithValue(t.Context(), addToContextGuardCtxKey{}, "should not see this"),
				http.MethodGet,
				"/test",
				nil,
			),
			wantResponseBody:       `{"context":"my context value"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"uses the current request if the guard returns nil": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					ctxVal, ok := r.Context().Value(addToContextGuardCtxKey{}).(addToContextGuard)
					if !ok {
						return nil, problem.BusinessRuleViolation(r.Request).WithDetail("ctxVal not set")
					}

					return httputil.OK(map[string]string{"context": string(ctxVal)})
				}),
			}, httputil.GuardFunc(func(_ *http.Request) (*http.Request, error) {
				return nil, nil
			})),
			request: httptest.NewRequestWithContext(
				context.WithValue(t.Context(), addToContextGuardCtxKey{}, addToContextGuard("my original context value")),
				http.MethodGet,
				"/test",
				nil,
			),
			wantResponseBody:       `{"context":"my original context value"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"writes a log when closing the request body errors": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NewResponse(http.StatusTeapot, nil), nil
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", errReadCloser("some error")),
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to close request body",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.StringValue("some error"),
				},
			}},
			wantResponseStatusCode: http.StatusTeapot,
		},
		"ignores a nil request body": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NewResponse(http.StatusTeapot, nil), nil
				}),
			},
			request: func() *http.Request {
				// A nil body is replaced during creation of a new http.NewRequest.
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Body = nil

				return req
			}(),
			wantResponseStatusCode: http.StatusTeapot,
		},
		"sets zero values when request params are missing and there is no validation": {
			endpoint: func() httputil.Endpoint {
				type params struct {
					Name          string `param:"query=name"`
					CorrelationID string `param:"header=X-Correlation-Id"`
					User          string `param:"path=user"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
						return httputil.OK(map[string]string{
							"name":          r.Params.Name,
							"correlationId": r.Params.CorrelationID,
							"user":          r.Params.User,
						})
					}),
				}
			}(),
			request:                httptest.NewRequest(http.MethodGet, "/test", nil),
			wantResponseBody:       `{"correlationId":"","name":"","user":""}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"sets default values when request params are missing and there is no validation": {
			endpoint: func() httputil.Endpoint {
				type params struct {
					Name          string `param:"query=name,default=some-name"`
					CorrelationID string `param:"header=X-Correlation-Id,default=some-correlation-id"`
					User          string `param:"path=user,default=some-user"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
						return httputil.OK(map[string]string{
							"name":          r.Params.Name,
							"correlationId": r.Params.CorrelationID,
							"user":          r.Params.User,
						})
					}),
				}
			}(),
			request:                httptest.NewRequest(http.MethodGet, "/test", nil),
			wantResponseBody:       `{"correlationId":"some-correlation-id","name":"some-name","user":"some-user"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"sets default values when request params are missing and there is validation requiring fields to be set": {
			endpoint: func() httputil.Endpoint {
				type params struct {
					Name          string `validate:"required" param:"query=name,default=some-name"`
					CorrelationID string `validate:"required" param:"header=X-Correlation-Id,default=some-correlation-id"`
					User          string `validate:"required" param:"path=user,default=some-user"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
						return httputil.OK(map[string]string{
							"name":          r.Params.Name,
							"correlationId": r.Params.CorrelationID,
							"user":          r.Params.User,
						})
					}),
				}
			}(),
			request:                httptest.NewRequest(http.MethodGet, "/test", nil),
			wantResponseBody:       `{"correlationId":"some-correlation-id","name":"some-name","user":"some-user"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"returns an error when request params are missing and there is validation but no defaults set": {
			endpoint: func() httputil.Endpoint {
				type params struct {
					Name          string `validate:"required" param:"query=name"`
					CorrelationID string `validate:"required" param:"header=X-Correlation-Id"`
					User          string `validate:"required" param:"path=user"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
						return httputil.OK(map[string]string{
							"name":          r.Params.Name,
							"correlationId": r.Params.CorrelationID,
							"user":          r.Params.User,
						})
					}),
				}
			}(),
			request: httptest.NewRequest(http.MethodGet, "/test", nil),
			wantResponseBody: problem.BadParameters(
				httptest.NewRequest(http.MethodGet, "/test", http.NoBody),
				problem.Parameter{Parameter: "name", Detail: "is required", Type: "query"},
				problem.Parameter{Parameter: "X-Correlation-Id", Detail: "is required", Type: "header"},
				problem.Parameter{Parameter: "user", Detail: "is required", Type: "path"},
			).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns an error when trying to unmarshal into a value that is not a struct": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.RequestParams[map[string]string]) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", nil),
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler params type is not a struct",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"type": slog.StringValue("map"),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"logs and returns an error when params extraction fails unexpectedly": {
			endpoint: func() httputil.Endpoint {
				type params struct {
					Name int `param:"query=name,default=not an int"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(_ httputil.RequestParams[params]) (*httputil.Response, error) {
						return httputil.NoContent()
					}),
				}
			}(),
			request: httptest.NewRequest(http.MethodGet, "/test", nil),
			wantLogs: []slogmem.RecordQuery{{
				Message: "Handler failed to decode params data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue(`setting field value: failed to convert parameter "default" to int: strconv.Atoi: parsing "not an int": invalid syntax`),
				},
			}},
			wantResponseBody:       problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"handles request types being set to any,any": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewHandler(func(_ httputil.Request[any, any]) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			wantResponseStatusCode: http.StatusNoContent,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
			server := httputil.NewServer(logger)

			if testCase.request == nil {
				testCase.request = httptest.NewRequest(http.MethodGet, "/test", nil)
			}

			response := httptest.NewRecorder()

			server.Register(
				httputil.EndpointGroup{testCase.endpoint}.
					WithMiddleware(func(next http.Handler) http.Handler {
						// We wrap with middleware here to ensure that the middleware doesn't block any
						// dependencies that the handler requires.
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							next.ServeHTTP(w, r)
						})
					})...,
			)
			server.ServeHTTP(response, testCase.request)

			if response.Result().StatusCode != testCase.wantResponseStatusCode {
				t.Errorf("response.Code = %d, want %d", response.Result().StatusCode, testCase.wantResponseStatusCode)
			}

			if diff := testutil.DiffJSON(testCase.wantResponseBody, response.Body.String()); diff != "" {
				t.Errorf("response.Body mismatch (-want +got):\n%s", diff)
			}

			if testCase.wantHeader != nil && !cmp.Equal(testCase.wantHeader, response.Result().Header) {
				t.Errorf("response.Header = %v, want: %v", response.Result().Header, testCase.wantHeader)
			}

			if len(testCase.wantLogs) != logs.Len() {
				t.Errorf("logs.Len() = %d, want: %d, logs: %+v", logs.Len(), len(testCase.wantLogs), logs.AsSliceOfNestedKeyValuePairs())
			}

			for _, query := range testCase.wantLogs {
				if ok, diff := logs.Contains(query); !ok {
					t.Errorf("logs do not contain query (-want +got): \n%s", diff)
				}
			}
		})
	}
}

type errReader string

var _ io.Reader = errReader("")

func (er errReader) Read(_ []byte) (int, error) {
	return 0, errors.New(string(er))
}

type errReadCloser string

var _ io.ReadCloser = errReadCloser("")

func (er errReadCloser) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (er errReadCloser) Close() error {
	return errors.New(string(er))
}

type dataFromCtxTransformer struct {
	TransformedData string `json:"data" header:"X-Data"`
}

var _ httputil.Transformer = &dataFromCtxTransformer{}

type ctxKeyData struct{}

func (dft *dataFromCtxTransformer) Transform(ctx context.Context) error {
	data := ctx.Value(ctxKeyData{})
	if data == nil {
		return errors.New("data was not set on the context")
	}

	if stringData, ok := data.(string); ok {
		dft.TransformedData = stringData
	}

	return nil
}

type errorTransformer struct{}

var _ httputil.Transformer = errorTransformer{}

func (errorTransformer) Transform(_ context.Context) error {
	return errors.New("some error")
}

type noopGuard struct{}

var _ httputil.Guard = noopGuard{}

func (noopGuard) Guard(r *http.Request) (*http.Request, error) {
	return r, nil
}

type errorGuard struct{}

var _ httputil.Guard = errorGuard{}

func (errorGuard) Guard(_ *http.Request) (*http.Request, error) {
	return nil, errors.New("some error")
}

type problemGuard struct{}

var _ httputil.Guard = problemGuard{}

func (problemGuard) Guard(r *http.Request) (*http.Request, error) {
	return nil, problem.BadRequest(r)
}

type addToContextGuard string

var _ httputil.Guard = addToContextGuard("")

type addToContextGuardCtxKey struct{}

func (ri addToContextGuard) Guard(r *http.Request) (*http.Request, error) {
	return r.WithContext(context.WithValue(r.Context(), addToContextGuardCtxKey{}, ri)), nil
}

func TestNewFormHandler(t *testing.T) {
	t.Parallel()

	t.Run("passes data validation errors to the action", func(t *testing.T) {
		t.Parallel()

		type request struct {
			Name  string `json:"name"  validate:"required"`
			Email string `json:"email" validate:"required,email"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(map[string]string{"errors": "captured"})
				},
			),
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if !capturedErrors.HasAny() {
			t.Fatal("expected errors to be present")
		}

		if capturedErrors.Data == nil {
			t.Fatal("expected Data error to be set")
		}

		if capturedErrors.Params != nil {
			t.Error("expected Params error to be nil")
		}

		// Check translated messages via Get.
		if got := capturedErrors.Get("name"); got != "is required" {
			t.Errorf("Get(\"name\") = %q, want %q", got, "is required")
		}

		if got := capturedErrors.Get("email"); got != "is required" {
			t.Errorf("Get(\"email\") = %q, want %q", got, "is required")
		}

		// Check All() returns all errors.
		all := capturedErrors.All()
		if len(all) != 2 {
			t.Errorf("All() returned %d entries, want 2", len(all))
		}
	})

	t.Run("passes decode errors to the action", func(t *testing.T) {
		t.Parallel()

		type request struct {
			Name string `json:"name"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(map[string]string{"status": "action called"})
				},
			),
		})

		// Invalid JSON.
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{invalid`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if !capturedErrors.HasAny() {
			t.Fatal("expected errors to be present")
		}

		if capturedErrors.Data == nil {
			t.Fatal("expected Data error to be set")
		}
	})

	t.Run("passes param errors to the action", func(t *testing.T) {
		t.Parallel()

		type params struct {
			Name string `validate:"required" param:"query=name"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodGet,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestParams[params]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(map[string]string{"status": "action called"})
				},
			),
		})

		// Missing required query param "name".
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if !capturedErrors.HasAny() {
			t.Fatal("expected errors to be present")
		}

		if capturedErrors.Params == nil {
			t.Fatal("expected Params error to be set")
		}

		if got := capturedErrors.Get("name"); got == "" {
			t.Error("expected Get(\"name\") to return an error message")
		}
	})

	t.Run("no errors when data is valid", func(t *testing.T) {
		t.Parallel()

		type request struct {
			Name string `json:"name" validate:"required"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(map[string]string{"name": r.Data.Name})
				},
			),
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"name":"Nick"}`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if capturedErrors.HasAny() {
			t.Error("expected no errors on valid request")
		}
	})

	t.Run("custom MessageFunc overrides default messages", func(t *testing.T) {
		t.Parallel()

		type request struct {
			Name string `json:"name" validate:"required"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(nil)
				},
				httputil.WithHandlerMessages(func(tag, _ string) string {
					if tag == "required" {
						return "ce champ est obligatoire"
					}

					return "valeur invalide"
				}),
			),
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if got := capturedErrors.Get("name"); got != "ce champ est obligatoire" {
			t.Errorf("Get(\"name\") = %q, want %q", got, "ce champ est obligatoire")
		}
	})

	t.Run("custom MessageFunc receives tag and param for parameterised validators", func(t *testing.T) {
		t.Parallel()

		type request struct {
			Name string `json:"name" validate:"required,min=3"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(nil)
				},
				httputil.WithHandlerMessages(func(tag, param string) string {
					if tag == "min" {
						return "doit contenir au moins " + param + " caractères"
					}

					return "valeur invalide"
				}),
			),
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"name":"ab"}`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if got := capturedErrors.Get("name"); got != "doit contenir au moins 3 caractères" {
			t.Errorf("Get(\"name\") = %q, want %q", got, "doit contenir au moins 3 caractères")
		}
	})

	t.Run("both data and params errors are captured simultaneously", func(t *testing.T) {
		t.Parallel()

		type (
			request struct {
				Age int `json:"age" validate:"required,min=1"`
			}
			params struct {
				ID string `validate:"required" param:"query=id"`
			}
		)

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.Request[request, params]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(nil)
				},
			),
		})

		// Missing query param "id" AND missing required "age" field.
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if capturedErrors.Params == nil {
			t.Error("expected Params error to be set")
		}

		if capturedErrors.Data == nil {
			t.Error("expected Data error to be set")
		}

		// Get should aggregate both.
		all := capturedErrors.All()
		if len(all) < 2 {
			t.Errorf("All() returned %d entries, want at least 2", len(all))
		}
	})

	t.Run("nested struct fields use dot-separated keys", func(t *testing.T) {
		t.Parallel()

		type address struct {
			City    string `json:"city"    validate:"required"`
			ZipCode string `json:"zipcode" validate:"required"`
		}

		type request struct {
			Name    string  `json:"name"    validate:"required"`
			Address address `json:"address"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(nil)
				},
			),
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"name":"Nick","address":{}}`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if !capturedErrors.HasAny() {
			t.Fatal("expected errors to be present")
		}

		// Nested fields should use dot notation.
		if got := capturedErrors.Get("address.city"); got != "is required" {
			t.Errorf("Get(\"address.city\") = %q, want %q", got, "is required")
		}

		if got := capturedErrors.Get("address.zipcode"); got != "is required" {
			t.Errorf("Get(\"address.zipcode\") = %q, want %q", got, "is required")
		}

		// Top-level field should not have errors (name was provided).
		if got := capturedErrors.Get("name"); got != "" {
			t.Errorf("Get(\"name\") = %q, want empty", got)
		}
	})

	t.Run("uses form struct tags for validation error field names", func(t *testing.T) {
		t.Parallel()

		type request struct {
			Email string `validate:"required,email" form:"email"`
			Name  string `validate:"required"       form:"full_name"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

		server := httputil.NewServer(logger,
			httputil.WithServerCodec(httputil.NewHTMLServerCodec(nil)),
		)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(nil)
				},
			),
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("email=notanemail"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		if !capturedErrors.HasAny() {
			t.Fatal("expected errors to be present")
		}

		// Field names should match the form tag, not the Go field name.
		if got := capturedErrors.Get("email"); got == "" {
			t.Error("expected Get(\"email\") to return an error message")
		}

		if got := capturedErrors.Get("full_name"); got != "is required" {
			t.Errorf("Get(\"full_name\") = %q, want %q", got, "is required")
		}

		// Go field names should NOT be used.
		if got := capturedErrors.Get("Email"); got != "" {
			t.Errorf("Get(\"Email\") = %q, want empty (should use form tag name)", got)
		}

		if got := capturedErrors.Get("Name"); got != "" {
			t.Errorf("Get(\"Name\") = %q, want empty (should use form tag name)", got)
		}
	})

	t.Run("json tag takes precedence over form tag for field names", func(t *testing.T) {
		t.Parallel()

		type request struct {
			Name string `json:"jsonName" validate:"required" form:"form_name"`
		}

		var capturedErrors httputil.BindErrors

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/test",
			Handler: httputil.NewFormHandler(
				func(r httputil.RequestData[request]) (*httputil.Response, error) {
					capturedErrors = r.Errors
					return httputil.OK(nil)
				},
			),
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
		res := httptest.NewRecorder()

		server.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.Code)
		}

		// json tag should win over form tag.
		if got := capturedErrors.Get("jsonName"); got != "is required" {
			t.Errorf("Get(\"jsonName\") = %q, want %q", got, "is required")
		}

		// form tag should NOT be used when json tag is present.
		if got := capturedErrors.Get("form_name"); got != "" {
			t.Errorf("Get(\"form_name\") = %q, want empty (json tag should take precedence)", got)
		}
	})
}
