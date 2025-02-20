package httputil_test

import (
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
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				})
			},
			wantHeader:             http.Header{"Content-Type": {"application/json"}},
			wantResponseStatusCode: http.StatusNoContent,
		},
		"the response content type is application/json when an error response is returned": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, errors.New("some error")
				})
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error from action",
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
				return httputil.NewJSONHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, problem.ServerError(r.Request)
				})
			},
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"code":"500-01","detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md"}`,
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
			wantResponseBody:       `{"code":"500-01","detail":"The server encountered an unexpected internal error","instance":"/test","status":500,"title":"Server Error","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/server-error.md"}`,
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns a bad request status code with errors if the payload is empty but request data is expected": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				type request struct {
					Name string `json:"name" validate:"required"`
				}

				return httputil.NewJSONHandler(func(_ httputil.RequestData[request]) (*httputil.Response, error) {
					return httputil.NoContent()
				})
			},
			requestBody:            strings.NewReader(""),
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"code":"400-01","detail":"The server received an unexpected empty request body","instance":"/test","status":400,"title":"Bad Request","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md"}`,
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"returns a bad request status code and logs a warning when the request body cannot be decoded as json": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestData[map[string]string]) (*httputil.Response, error) {
					return httputil.NoContent()
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
			wantResponseBody:       `{"code":"400-01","detail":"The request is invalid or malformed","instance":"/test","status":400,"title":"Bad Request","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md"}`,
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

				return httputil.NewJSONHandler(func(_ httputil.RequestData[request]) (*httputil.Response, error) {
					return httputil.NoContent()
				})
			},
			requestBody:            strings.NewReader("{}"),
			wantHeader:             http.Header{"Content-Type": {"application/problem+json"}},
			wantResponseBody:       `{"code":"422-02","detail":"The request data violated one or more validation constraints","instance":"/test","status":422,"title":"Constraint Violation","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md","violations":[{"detail":"thing is required","pointer":"/inner/thing"}]}`,
			wantResponseStatusCode: http.StatusUnprocessableEntity,
		},
		"the request body can be read again in the action after it has been decoded into the request data type": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(r httputil.RequestData[map[string]string]) (*httputil.Response, error) {
					bytes, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("failed to read r.Body, err: %v", err)
					}

					if diff := testutil.DiffJSON(string(bytes), `{"hello":"world"}`); diff != "" {
						t.Errorf("r.Body mismatch (-want +got):\n%s", diff)
					}

					return httputil.NoContent()
				})
			},
			requestBody:            strings.NewReader(`{"hello":"world"}`),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"the request body is mapped to the requests data": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(r httputil.RequestData[map[string]string]) (*httputil.Response, error) {
					if r.Data["hello"] != "world" {
						t.Errorf("r.Data[\"hello\"] = %v, want: world", r.Data["hello"])
					}

					return httputil.NoContent()
				})
			},
			requestBody:            strings.NewReader(`{"hello":"world"}`),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"an internal server error is returned and a log is written when a generic error is returned": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, errors.New("some error")
				})
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error from action",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"status code is used from the response on successful request": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Accepted(nil)
				})
			},
			wantResponseStatusCode: http.StatusAccepted,
		},
		"response data is encoded as json in the body": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.OK(map[string]string{"hello": "world"})
				})
			},
			wantResponseBody:       `{"hello":"world"}`,
			wantResponseStatusCode: http.StatusOK,
		},
		"logs a warning when the response body cannot be encoded as json": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Created(map[string]chan int{"chan": make(chan int)})
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
		"only handles the error case when both an error and a response is returned from the action": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NewResponse(http.StatusNoContent, nil), errors.New("some error")
				})
			},
			wantHeader: http.Header{"Content-Type": {"application/json"}},
			wantLogs: []slogmem.RecordQuery{{
				Message: "JSON handler received an unhandled error from action",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("some error"),
				},
			}},
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"redirects the request when a redirect response is returned": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.Redirect(http.StatusPermanentRedirect, "http://example.com")
				})
			},
			wantHeader:             http.Header{"Content-Type": []string{"application/json"}, "Location": []string{"http://example.com"}},
			wantResponseStatusCode: http.StatusPermanentRedirect,
		},
		"allows writing to the response writer directly": {
			handler: func(t *testing.T) http.Handler {
				t.Helper()
				return httputil.NewJSONHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
					r.ResponseWriter.Header().Set("X-Correlation-Id", "some-random-id")
					r.ResponseWriter.WriteHeader(http.StatusTeapot)

					return nil, nil
				})
			},
			wantHeader:             http.Header{"Content-Type": []string{"application/json"}, "X-Correlation-Id": []string{"some-random-id"}},
			wantResponseStatusCode: http.StatusTeapot,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
			server := httputil.NewServer(logger)

			request := httptest.NewRequest(http.MethodGet, "/test", testCase.requestBody)
			response := httptest.NewRecorder()

			handler := httputil.NewJSONHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
				return httputil.OK(nil)
			})
			if testCase.handler != nil {
				handler = testCase.handler(t)
			}

			server.Register(httputil.Endpoint{Method: http.MethodGet, Path: "/test", Handler: handler})
			server.ServeHTTP(response, request)

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
