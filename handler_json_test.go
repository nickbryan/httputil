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

func TestNewJSONHandler(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		request                *http.Request
		endpoint               httputil.Endpoint
		wantLogs               []slogmem.RecordQuery
		wantHeader             http.Header
		wantResponseBody       string
		wantResponseStatusCode int
	}{
		"the response content type is application/json when a successful response is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			wantHeader:             http.Header{"Content-Type": {"application/json"}},
			wantResponseStatusCode: http.StatusNoContent,
		},
		"the response content type is application/json when an error response is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, errors.New("some error")
				}),
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling action: some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"the response content type is application/problem+json when a problem response is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, problem.ServerError(r.Request)
				}),
			},
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"code":"500-01","detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns an internal server error status code and logs a warning when the request body cannot be read": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", errReader("the request body was invalid")),
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to read request body",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("the request body was invalid"),
				},
			}},
			wantResponseBody:       `{"code":"500-01","detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns a bad request status code with errors if the payload is empty but request data is expected": {
			endpoint: func() httputil.Endpoint {
				type request struct {
					Name string `json:"name" validate:"required"`
				}

				return httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewJSONHandler(func(_ httputil.RequestData[request]) (*httputil.Response, error) {
						return httputil.NoContent()
					}),
				}
			}(),
			request:                httptest.NewRequest(http.MethodGet, "/test", strings.NewReader("")),
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"code":"400-01","detail":"The server received an unexpected empty request body","instance":"/test","status":400,"title":"Bad Request","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md"}`,
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns a bad request status code and logs a warning when the request body cannot be decoded as json": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestData[map[string]string]) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			},
			request:    httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{`)),
			wantHeader: http.Header{"Content-Type": {"application/problem+json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to decode request data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("unexpected end of JSON input"),
				},
			}},
			wantResponseBody:       `{"code":"400-01","detail":"The request is invalid or malformed","instance":"/test","status":400,"title":"Bad Request","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md"}`,
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
					Handler: httputil.NewJSONHandler(func(_ httputil.RequestData[request]) (*httputil.Response, error) {
						return httputil.NoContent()
					}),
				}
			}(),
			request:                httptest.NewRequest(http.MethodGet, "/test", strings.NewReader("{}")),
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"code":"422-02","detail":"The request data violated one or more validation constraints","instance":"/test","status":422,"title":"Constraint Violation","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md","violations":[{"detail":"thing is required","pointer":"#/inner/thing"}]}`,
			wantResponseStatusCode: http.StatusUnprocessableEntity,
		},
		"the request body can be read again in the action after it has been decoded into the request data type": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(r httputil.RequestData[map[string]string]) (*httputil.Response, error) {
					bytes, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("failed to read r.Body, err: %v", err)
					}

					if diff := testutil.DiffJSON(string(bytes), `{"hello":"world"}`); diff != "" {
						t.Errorf("r.Body mismatch (-want +got):\n%s", diff)
					}

					return httputil.NoContent()
				}),
			},
			request:                httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{"hello":"world"}`)),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"the request body is mapped to the requests data": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(r httputil.RequestData[map[string]string]) (*httputil.Response, error) {
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
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, errors.New("some error")
				}),
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling action: some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"status code is used from the response on successful request": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Accepted(nil)
				}),
			},
			wantResponseStatusCode: http.StatusAccepted,
		},
		"response data is encoded as json in the body": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
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
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Created(map[string]chan int{"chan": make(chan int)})
				}),
			},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to encode response data",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("json: unsupported type: chan int"),
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
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NewResponse(http.StatusNoContent, nil), errors.New("some error") //nolint:nilnil // Requires both to be set for test.
				}),
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling action: some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"redirects the request when a redirect response is returned": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Redirect(http.StatusPermanentRedirect, "http://example.com")
				}),
			},
			wantHeader:             http.Header{"Content-Type": []string{"application/json"}, "Location": []string{"http://example.com"}},
			wantResponseStatusCode: http.StatusPermanentRedirect,
		},
		"allows writing to the response writer directly": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					r.ResponseWriter.Header().Set("X-Correlation-Id", "some-random-id")
					r.ResponseWriter.WriteHeader(http.StatusTeapot)

					return httputil.NothingToHandle()
				}),
			},
			wantHeader:             http.Header{"Content-Type": []string{"application/json"}, "X-Correlation-Id": []string{"some-random-id"}},
			wantResponseStatusCode: http.StatusTeapot,
		},
		"request data is transformed before the action is called": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(r httputil.RequestData[dataFromCtxTransformer]) (*httputil.Response, error) {
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
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestData[errorTransformer]) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"data": "should not be returned"})
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{"data":"some-data"}`)),
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to transform request data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("transforming data: some error"),
				},
			}},
			wantResponseBody:       `{"code":"500-01","detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"params data is transformed before the action is called": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(r httputil.RequestParams[dataFromCtxTransformer]) (*httputil.Response, error) {
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
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestParams[errorTransformer]) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"data": "should not be returned"})
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{"data":"some-data"}`)),
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to transform params data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("transforming data: some error"),
				},
			}},
			wantResponseBody:       `{"code":"500-01","detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"response data is transformed after the action is called": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
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
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.OK(errorTransformer{})
				}),
			},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to transform response data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("transforming data: some error"),
				},
			}},
			wantResponseBody:       `{"code":"500-01","detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns the response when a guard is set as nil": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, nil),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"returns the response when the guard returns nil": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, nilnilGuard{}),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"returns and logs an error when the guard returns an error": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, errorGuard{}),
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling guard: some error"),
				},
			}},
			wantResponseBody:       "",
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns a problem error when the guard returns an problem error type": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, problemGuard{}),
			wantResponseBody:       `{"code":"400-01","detail":"The request is invalid or malformed","instance":"/test","status":400,"title":"Bad Request","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md"}`,
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns a response when the guard returns a response": {
			endpoint: httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				}),
			}, redirectToCtxGuard{}),
			request: httptest.NewRequestWithContext(
				context.WithValue(t.Context(), ctxKeyRedirect{}, "http://example.com"),
				http.MethodGet,
				"/test",
				nil,
			),
			wantHeader:             http.Header{"Content-Type": []string{"application/json"}, "Location": []string{"http://example.com"}},
			wantResponseStatusCode: http.StatusPermanentRedirect,
		},
		"writes a log when closing the request body errors": {
			endpoint: httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NewResponse(http.StatusTeapot, nil), nil
				}),
			},
			request: httptest.NewRequest(http.MethodGet, "/test", errReadCloser("some error")),
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to close request body",
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
				Handler: httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NewResponse(http.StatusTeapot, nil), nil
				}),
			},
			request: func() *http.Request {
				// A nil body is replaced during creation of a new http.Request.
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Body = nil

				return req
			}(),
			wantResponseStatusCode: http.StatusTeapot,
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

			if response.Code != testCase.wantResponseStatusCode {
				t.Errorf("response.Code = %d, want %d", response.Code, testCase.wantResponseStatusCode)
			}

			if diff := testutil.DiffJSON(testCase.wantResponseBody, response.Body.String()); diff != "" {
				t.Errorf("response.Body mismatch (-want +got):\n%s", diff)
			}

			if testCase.wantHeader != nil && !cmp.Equal(testCase.wantHeader, response.Header()) {
				t.Errorf("response.Header = %v, want: %v", response.Header(), testCase.wantHeader)
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

type funcGuard func(r *http.Request) (*httputil.Response, error)

func (f funcGuard) Guard(r *http.Request) (*httputil.Response, error) {
	return f(r)
}

type nilnilGuard struct{}

var _ httputil.Guard = nilnilGuard{}

func (nilnilGuard) Guard(_ *http.Request) (*httputil.Response, error) {
	return httputil.NothingToHandle()
}

type errorGuard struct{}

var _ httputil.Guard = errorGuard{}

func (errorGuard) Guard(_ *http.Request) (*httputil.Response, error) {
	return nil, errors.New("some error")
}

type problemGuard struct{}

var _ httputil.Guard = problemGuard{}

func (problemGuard) Guard(r *http.Request) (*httputil.Response, error) {
	return nil, problem.BadRequest(r)
}

type redirectToCtxGuard struct{}

var _ httputil.Guard = redirectToCtxGuard{}

type ctxKeyRedirect struct{}

func (redirectToCtxGuard) Guard(r *http.Request) (*httputil.Response, error) {
	location := r.Context().Value(ctxKeyRedirect{})
	if location == nil {
		return nil, errors.New("location was not set on the context")
	}

	locationString, ok := location.(string)
	if !ok {
		return nil, errors.New("location was not a string")
	}

	return httputil.Redirect(http.StatusPermanentRedirect, locationString)
}
