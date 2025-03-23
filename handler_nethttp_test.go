package httputil_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/internal/testutil"
	"github.com/nickbryan/httputil/problem"
	"github.com/nickbryan/httputil/problem/problemtest"
)

func TestNewNetHTTPHandler(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		request                *http.Request
		endpoint               httputil.Endpoint
		wantLogs               []slogmem.RecordQuery
		wantResponseBody       string
		wantResponseStatusCode int
	}{
		"returns the response when a interceptor is set as nil": {
			endpoint: httputil.NewEndpointWithRequestInterceptor(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}),
			}, nil),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"returns the response when the interceptor does not block the handler": {
			endpoint: httputil.NewEndpointWithRequestInterceptor(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}),
			}, noopRequestInterceptor{}),
			wantResponseStatusCode: http.StatusNoContent,
		},
		"returns and logs an error when the interceptor blocks the handler by returning an error": {
			endpoint: httputil.NewEndpointWithRequestInterceptor(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}),
			}, errorRequestInterceptor{}),
			wantLogs: []slogmem.RecordQuery{{
				Message: "net/http handler received an unhandled error",
				Level:   slog.LevelError,
				Attrs: map[string]slog.Value{
					"error": slog.AnyValue("calling request interceptor: some error"),
				},
			}},
			wantResponseBody:       problem.ServerError(problemtest.NewRequest("/test")).Error(),
			wantResponseStatusCode: http.StatusInternalServerError,
		},
		"returns a problem error when the interceptor blocks the handler by returning a problem error type": {
			endpoint: httputil.NewEndpointWithRequestInterceptor(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}),
			}, problemRequestInterceptor{}),
			wantResponseBody:       problem.BadRequest(problemtest.NewRequest("/test")).Error(),
			wantResponseStatusCode: http.StatusBadRequest,
		},
		"allows the interceptor to add to the request context which is passed to the handler for consumption": {
			endpoint: httputil.NewEndpointWithRequestInterceptor(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewNetHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctxVal, ok := r.Context().Value(addToContextRequestInterceptorCtxKey{}).(addToContextRequestInterceptor)
					if !ok {
						ctxVal = "ctxVal not set"
					}

					if _, err := w.Write([]byte(ctxVal)); err != nil {
						panic(err)
					}
				}),
			}, addToContextRequestInterceptor("my context value")),
			request: httptest.NewRequestWithContext(
				context.WithValue(t.Context(), addToContextRequestInterceptorCtxKey{}, "should not see this"),
				http.MethodGet,
				"/test",
				nil,
			),
			wantResponseBody:       `my context value`,
			wantResponseStatusCode: http.StatusOK,
		},
		"uses the current request if the interceptor returns nil": {
			endpoint: httputil.NewEndpointWithRequestInterceptor(httputil.Endpoint{
				Method: http.MethodGet,
				Path:   "/test",
				Handler: httputil.NewNetHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctxVal, ok := r.Context().Value(addToContextRequestInterceptorCtxKey{}).(addToContextRequestInterceptor)
					if !ok {
						ctxVal = "ctxVal not set"
					}

					if _, err := w.Write([]byte(ctxVal)); err != nil {
						panic(err)
					}
				}),
			}, httputil.RequestInterceptorFunc(func(_ *http.Request) (*http.Request, error) {
				return nil, nil //nolint:nilnil // Required for test case.
			})),
			request: httptest.NewRequestWithContext(
				context.WithValue(t.Context(), addToContextRequestInterceptorCtxKey{}, addToContextRequestInterceptor("my original context value")),
				http.MethodGet,
				"/test",
				nil,
			),
			wantResponseBody:       `my original context value`,
			wantResponseStatusCode: http.StatusOK,
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
