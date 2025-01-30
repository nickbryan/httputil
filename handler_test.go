package httputil_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/go-cmp/cmp"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/internal/testutil"
	"github.com/nickbryan/httputil/problem"
	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"
)

type errReader string

func (er errReader) Read(_ []byte) (int, error) {
	return 0, errors.New(string(er))
}

func TestNewJSONHandler(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		handler                func(t *testing.T) http.Handler
		requestBody            io.Reader
		wantLogs               []slogmem.RecordQuery
		wantHeader             http.Header
		wantResponseBody       string
		wantResponseStatusCode int
	}{
		"the response content type is application/json when a successful response is returned": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					return httputil.NewNoContentResponse(), nil
				})
			},
			wantHeader:             http.Header{"Content-Type": {"application/json"}},
			wantResponseStatusCode: http.StatusNoContent,
		},
		"the response content type is application/json when an error response is returned": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					return nil, errors.New("some error")
				})
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error from inner handler",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"the response content type is application/problem+json when a problem response is returned": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(r httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					return nil, problem.ServerError(r.Request)
				})
			},
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://pkg.go.dev/github.com/nickbryan/httputil/problem#ServerError"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns an internal server error status code and logs a warning when the request body cannot be read": {
			requestBody: errReader("the request body was invalid"),
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to read request body",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("the request body was invalid"),
				},
			}},
			wantResponseBody:       `{"detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://pkg.go.dev/github.com/nickbryan/httputil/problem#ServerError"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns a bad request status code with errors if the payload is empty but request data is expected": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				type request struct {
					Name string `json:"name" validate:"required"`
				}

				return httputil.NewJSONHandler(func(_ httputil.Request[request]) (*httputil.ResponseNoBody, error) {
					return httputil.NewNoContentResponse(), nil
				})
			},
			requestBody:            strings.NewReader(""),
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"detail":"The server received an unexpected empty request body","instance":"/test","status":400,"title":"Bad Request","type":"https://pkg.go.dev/github.com/nickbryan/httputil/problem#BadRequest"}`,
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns a bad request status code and logs a warning when the request body cannot be decoded as json": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.Request[map[string]string]) (*httputil.ResponseNoBody, error) {
					return httputil.NewNoContentResponse(), nil
				})
			},
			requestBody: strings.NewReader(`{`),
			wantHeader:  http.Header{"Content-Type": {"application/problem+json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler failed to decode request data",
				Level:   slog.LevelWarn,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("unexpected end of JSON input"),
				},
			}},
			wantResponseBody:       `{"detail":"The request is invalid or malformed","instance":"/test","status":400,"title":"Bad Request","type":"https://pkg.go.dev/github.com/nickbryan/httputil/problem#BadRequest"}`,
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns an unprocessable entity request status code with errors if the payload fails validation": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				type inner struct {
					Thing string `json:"thing" validate:"required"`
				}

				type request struct {
					Name  string `json:"name"`
					Inner inner  `json:"inner"`
				}

				return httputil.NewJSONHandler(func(_ httputil.Request[request]) (*httputil.ResponseNoBody, error) {
					return httputil.NewNoContentResponse(), nil
				})
			},
			requestBody:            strings.NewReader("{}"),
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"detail":"The request data violated one or more validation constraints","instance":"/test","status":422,"title":"Constraint Violation","type":"https://pkg.go.dev/github.com/nickbryan/httputil/problem#ConstraintViolation","violations":[{"detail":"required","pointer":"/inner/thing"}]}`,
			wantResponseStatusCode: http.StatusUnprocessableEntity,
		},
		"the request body can be read again in the handler after it has been decoded into the request data type": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(r httputil.Request[map[string]string]) (*httputil.ResponseNoBody, error) {
					bytes, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("failed to read r.Body, err: %v", err)
					}

					if diff := testutil.DiffJSON(string(bytes), `{"hello":"world"}`); diff != "" {
						t.Errorf("r.Body mismatch (-want +got):\n%s", diff)
					}

					return httputil.NewNoContentResponse(), nil
				})
			},
			requestBody:            strings.NewReader(`{"hello":"world"}`),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"the request body is mapped to the requests data": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(r httputil.Request[map[string]string]) (*httputil.ResponseNoBody, error) {
					if r.Data["hello"] != "world" {
						t.Errorf("r.Data[\"hello\"] = %v, want: world", r.Data["hello"])
					}

					return httputil.NewNoContentResponse(), nil
				})
			},
			requestBody:            strings.NewReader(`{"hello":"world"}`),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"an internal server error is returned and a log is written when a generic error is returned": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					return nil, errors.New("some error")
				})
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error from inner handler",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"custom headers are set in the response on successful request": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					resp := httputil.NewNoContentResponse()
					resp.Header.Set("My-Header", "value")

					return resp, nil
				})
			},
			wantHeader:             http.Header{"Content-Type": {"application/json"}, "My-Header": {"value"}},
			wantResponseStatusCode: http.StatusNoContent,
		},
		"status code is used from the response on successful request": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					return httputil.NewResponse(http.StatusAccepted, struct{}{}), nil
				})
			},
			wantResponseStatusCode: http.StatusAccepted,
		},
		"response data is encoded as json in the body": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.Response[map[string]string], error) {
					return httputil.NewResponse(http.StatusOK, map[string]string{"hello": "world"}), nil
				})
			},
			wantResponseBody:       `{"hello":"world"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"logs a warning when the response body cannot be encoded as json": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.Response[map[string]chan int], error) {
					return httputil.NewResponse(http.StatusCreated, map[string]chan int{"chan": make(chan int)}), nil
				})
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
		"only the error case is handled when both an error and a response is returned from the handler": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					return httputil.NewNoContentResponse(), errors.New("some error")
				})
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error from inner handler",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)

			request := httptest.NewRequest(http.MethodGet, "/test", testCase.requestBody)
			response := httptest.NewRecorder()

			handler := httputil.NewJSONHandler(
				func(_ httputil.RequestNoBody) (*httputil.ResponseNoBody, error) {
					return httputil.NewResponse(http.StatusOK, struct{}{}), nil
				},
			)
			if testCase.handler != nil {
				handler = testCase.handler(t)
			}

			handler = withDependencies(t, handler, logger)
			handler.ServeHTTP(response, request)

			if response.Code != testCase.wantResponseStatusCode {
				t.Errorf("response.Code = %d, want %d", response.Code, testCase.wantResponseStatusCode)
			}

			if diff := testutil.DiffJSON(testCase.wantResponseBody, strings.TrimSpace(response.Body.String())); diff != "" {
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

func withDependencies(t *testing.T, handler http.Handler, logger *slog.Logger) http.Handler {
	t.Helper()

	if logSetter, ok := handler.(interface{ SetLogger(l *slog.Logger) }); ok {
		logSetter.SetLogger(logger)
	} else {
		t.Fatal("unable to set logger on handler")
	}

	if validatorSetter, ok := handler.(interface{ SetValidator(v *validator.Validate) }); ok {
		validatorSetter.SetValidator(httputil.NewValidator())
	} else {
		t.Fatal("unable to set validator on handler")
	}

	return handler
}
