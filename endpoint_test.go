package httputil_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/nickbryan/slogutil"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

func TestEndpointGroupWithGuard(t *testing.T) {
	t.Parallel()

	type statusInCtxKey struct{}

	teapotInContextInterceptor := httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
		return r.WithContext(context.WithValue(r.Context(), statusInCtxKey{}, http.StatusTeapot)), nil
	})

	statusFromContextHandler := httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
		ctxVal, ok := r.Context().Value(statusInCtxKey{}).(int)
		if !ok {
			return nil, problem.BusinessRuleViolation(r.Request).WithDetail("ctxVal not set")
		}

		return httputil.NewResponse(ctxVal, nil), nil
	})

	type testRequest struct {
		path         string
		method       string
		expectedCode int
	}

	testCases := map[string]struct {
		endpoints httputil.EndpointGroup
		guards    []httputil.Guard
		requests  []testRequest
	}{
		"nil guard does nothing": {
			endpoints: httputil.EndpointGroup{
				httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
						return httputil.NewResponse(http.StatusOK, nil), nil
					}),
				},
			},
			guards: nil,
			requests: []testRequest{
				{path: "/test", method: http.MethodGet, expectedCode: http.StatusOK},
			},
		},
		"multiple nil guards do nothing": {
			endpoints: httputil.EndpointGroup{
				httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
						return httputil.NewResponse(http.StatusOK, nil), nil
					}),
				},
			},
			guards: []httputil.Guard{nil, nil},
			requests: []testRequest{
				{path: "/test", method: http.MethodGet, expectedCode: http.StatusOK},
			},
		},
		"single guard modifies request": {
			endpoints: httputil.EndpointGroup{
				httputil.Endpoint{
					Method:  http.MethodGet,
					Path:    "/test",
					Handler: statusFromContextHandler,
				},
			},
			guards: []httputil.Guard{teapotInContextInterceptor},
			requests: []testRequest{
				{path: "/test", method: http.MethodGet, expectedCode: http.StatusTeapot},
			},
		},
		"single guard applies to multiple endpoints": {
			endpoints: httputil.EndpointGroup{
				httputil.Endpoint{
					Method:  http.MethodGet,
					Path:    "/testA",
					Handler: statusFromContextHandler,
				},
				httputil.Endpoint{
					Method:  http.MethodGet,
					Path:    "/testB",
					Handler: statusFromContextHandler,
				},
			},
			guards: []httputil.Guard{teapotInContextInterceptor},
			requests: []testRequest{
				{path: "/testA", method: http.MethodGet, expectedCode: http.StatusTeapot},
				{path: "/testB", method: http.MethodGet, expectedCode: http.StatusTeapot},
			},
		},
		"multiple guards are stacked": {
			endpoints: httputil.EndpointGroup{
				httputil.Endpoint{
					Method:  http.MethodGet,
					Path:    "/testA",
					Handler: statusFromContextHandler,
				},
				httputil.Endpoint{
					Method:  http.MethodGet,
					Path:    "/testB",
					Handler: statusFromContextHandler,
				},
			},
			guards: []httputil.Guard{
				noopGuard{},
				teapotInContextInterceptor,
			},
			requests: []testRequest{
				{path: "/testA", method: http.MethodGet, expectedCode: http.StatusTeapot},
				{path: "/testB", method: http.MethodGet, expectedCode: http.StatusTeapot},
			},
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			interceptedEndpoints := testCase.endpoints
			for _, guard := range testCase.guards {
				interceptedEndpoints = interceptedEndpoints.WithGuard(guard)
			}

			logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
			server := httputil.NewServer(logger)
			server.Register(interceptedEndpoints...)

			for _, req := range testCase.requests {
				httpReq := httptest.NewRequest(req.method, req.path, nil)
				resp := httptest.NewRecorder()
				server.ServeHTTP(resp, httpReq)

				if resp.Code != req.expectedCode {
					t.Errorf("path %q: incorrect status code got: %d, want: %d", req.path, resp.Code, req.expectedCode)
				}
			}
		})
	}
}

func TestEndpointGroupWithMiddleware(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}

	injectContextValueMiddleware := httputil.MiddlewareFunc(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxKey{}, "value")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	newTestHandler := func(t *testing.T) http.HandlerFunc {
		t.Helper()

		return func(w http.ResponseWriter, r *http.Request) {
			val, ok := r.Context().Value(ctxKey{}).(string)
			if !ok {
				t.Fatal("unable to cast context value to string")
			}

			if _, err := w.Write([]byte(val)); err != nil {
				t.Fatalf("unable to write response, err: %+v", err)
			}
		}
	}

	endpointHasMiddleware := func(endpoint httputil.Endpoint) bool {
		request := httptest.NewRequest(http.MethodGet, "/users", nil)
		responseBody := bytes.Buffer{}
		response := httptest.NewRecorder()
		response.Body = &responseBody

		endpoint.Handler.ServeHTTP(response, request)

		return responseBody.String() == "value"
	}

	t.Run("returns nothing when no endpoints are passed and middleware is nil", func(t *testing.T) {
		t.Parallel()

		endpoints := httputil.EndpointGroup{}.WithMiddleware(nil)

		if len(endpoints) != 0 {
			t.Errorf("expected len(endpoints) = 0, got: %d", len(endpoints))
		}
	})

	t.Run("returns nothing when no endpoints are passed and middleware is not nil", func(t *testing.T) {
		t.Parallel()

		endpoints := httputil.EndpointGroup{}.WithMiddleware(injectContextValueMiddleware)

		if len(endpoints) != 0 {
			t.Errorf("expected len(endpoints) = 0, got: %d", len(endpoints))
		}
	})

	t.Run("returns the endpoints when middleware is nil", func(t *testing.T) {
		t.Parallel()

		endpoints := httputil.EndpointGroup{
			{Method: http.MethodGet, Path: "/users", Handler: nil},
			{Method: http.MethodPost, Path: "/users", Handler: nil},
		}

		endpointsWithMiddleware := endpoints.WithMiddleware(nil)

		if len(endpointsWithMiddleware) != len(endpoints) {
			t.Errorf("expected len(endpoints) = %d, got: %d", len(endpoints), len(endpointsWithMiddleware))
		}

		if diff := cmp.Diff(endpoints, endpointsWithMiddleware, cmpopts.IgnoreInterfaces(struct{ httputil.Guard }{})); diff != "" {
			t.Errorf("returned endpoints are not the same as the passed endpoints, diff: %s", diff)
		}
	})

	t.Run("returns an endpoint with middleware", func(t *testing.T) {
		t.Parallel()

		endpointsWithMiddleware := httputil.EndpointGroup{{
			Method:  http.MethodPost,
			Path:    "/users",
			Handler: httputil.WrapNetHTTPHandler(newTestHandler(t)),
		}}.WithMiddleware(injectContextValueMiddleware)

		if len(endpointsWithMiddleware) != 1 {
			t.Fatalf("expected len(endpoints) = 1, got: %d", len(endpointsWithMiddleware))
		}

		if !endpointHasMiddleware(endpointsWithMiddleware[0]) {
			t.Error("the handler was not wrapped by the MiddlewareFunc")
		}
	})

	t.Run("returns multiple endpoints with middleware", func(t *testing.T) {
		t.Parallel()

		endpoints := httputil.EndpointGroup{
			{Method: http.MethodGet, Path: "/users", Handler: httputil.WrapNetHTTPHandler(newTestHandler(t))},
			{Method: http.MethodPost, Path: "/users", Handler: httputil.WrapNetHTTPHandler(newTestHandler(t))},
		}

		endpointsWithMiddleware := endpoints.WithMiddleware(injectContextValueMiddleware)

		if len(endpointsWithMiddleware) != len(endpoints) {
			t.Errorf("expected len(endpoints) = %d, got: %d", len(endpoints), len(endpointsWithMiddleware))
		}

		for _, endpoint := range endpointsWithMiddleware {
			if !endpointHasMiddleware(endpoint) {
				t.Error("the handler was not wrapped by the MiddlewareFunc")
			}
		}
	})
}

func TestEndpointGroupWithPrefix(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		endpointsPaths    []string
		prefix            string
		wantEndpointPaths []string
	}{
		"no endpoints are returned when no endpoints are passed": {
			endpointsPaths:    []string{},
			prefix:            "/api",
			wantEndpointPaths: []string{},
		},
		"a single endpoint is prefixed": {
			endpointsPaths:    []string{"/users"},
			prefix:            "/api",
			wantEndpointPaths: []string{"/api/users"},
		},
		"multiple endpoints are prefixed": {
			endpointsPaths:    []string{"/users", "/accounts"},
			prefix:            "/api",
			wantEndpointPaths: []string{"/api/users", "/api/accounts"},
		},
		"no prefix is added when prefix is empty": {
			endpointsPaths:    []string{"/users"},
			prefix:            "",
			wantEndpointPaths: []string{"/users"},
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			var endpoints httputil.EndpointGroup
			for _, path := range testCase.endpointsPaths {
				endpoints = append(endpoints, httputil.Endpoint{
					Method:  http.MethodGet,
					Path:    path,
					Handler: nil,
				})
			}

			endpoints = endpoints.WithPrefix(testCase.prefix)

			if len(testCase.wantEndpointPaths) != len(endpoints) {
				t.Fatalf("number of returned endpoints (%d) != wantEndpointPaths (%d)", len(endpoints), len(testCase.wantEndpointPaths))
			}

			for i, endpoint := range endpoints {
				if endpoint.Path != testCase.wantEndpointPaths[i] {
					t.Errorf("incorrect endpoint path: want: %s, got: %s", testCase.wantEndpointPaths[i], endpoint.Path)
				}
			}
		})
	}
}

func TestGuardStack(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}

	valueContextInterceptor := func(value string) httputil.Guard {
		return httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
			return r.WithContext(context.WithValue(r.Context(), ctxKey{}, value)), nil
		})
	}

	testCases := map[string]struct {
		guardStack    httputil.GuardStack
		wantReq       bool
		wantReqCtxVal string
		wantErr       error
	}{
		"nil stack returns request and nil error": {
			guardStack: nil,
			wantReq:    true,
			wantErr:    nil,
		},
		"no handlers in stack returns request and nil error": {
			guardStack: httputil.GuardStack{},
			wantReq:    true,
			wantErr:    nil,
		},
		"single guard: returns request and nil error": {
			guardStack: httputil.GuardStack{noopGuard{}},
			wantReq:    true,
			wantErr:    nil,
		},
		"single guard: returns non-nil request and nil error": {
			guardStack:    httputil.GuardStack{valueContextInterceptor("some value")},
			wantReq:       true,
			wantReqCtxVal: "some value",
			wantErr:       nil,
		},
		"single guard: returns nil request and non-nil error": {
			guardStack: httputil.GuardStack{
				httputil.GuardFunc(func(_ *http.Request) (*http.Request, error) {
					return nil, errors.New("some error")
				}),
			},
			wantReq: false,
			wantErr: errors.New("some error"),
		},
		"multiple guards: request is passed through all guards so they can add to context if required": {
			guardStack: httputil.GuardStack{
				valueContextInterceptor("some value"),
				httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
					ctxVal, ok := r.Context().Value(ctxKey{}).(string)
					if !ok {
						return nil, errors.New("ctxKey value not set")
					}

					return r.WithContext(context.WithValue(r.Context(), ctxKey{}, ctxVal+" that was appended to")), nil
				}),
			},
			wantReq:       true,
			wantReqCtxVal: "some value that was appended to",
			wantErr:       nil,
		},
		"multiple guards: first returns nil request and non-nil error, skips subsequent guards": {
			guardStack: httputil.GuardStack{
				httputil.GuardFunc(func(_ *http.Request) (*http.Request, error) {
					return nil, errors.New("first handler error")
				}),
				httputil.GuardFunc(func(_ *http.Request) (*http.Request, error) {
					return nil, errors.New("should not be called")
				}),
			},
			wantReq: false,
			wantErr: errors.New("first handler error"),
		},
		"multiple guards: all return request and nil error": {
			guardStack: httputil.GuardStack{noopGuard{}, noopGuard{}},
			wantReq:    true,
			wantErr:    nil,
		},
		"multiple guards: second guard returns non-nil response and nil error": {
			guardStack: httputil.GuardStack{
				noopGuard{},
				valueContextInterceptor("some value"),
			},
			wantReq:       true,
			wantReqCtxVal: "some value",
			wantErr:       nil,
		},
		"multiple guards: last returns nil request and non-nil error": {
			guardStack: httputil.GuardStack{
				noopGuard{},
				httputil.GuardFunc(func(_ *http.Request) (*http.Request, error) {
					return nil, errors.New("last guard error")
				}),
			},
			wantReq: false,
			wantErr: errors.New("last guard error"),
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			req, err := testCase.guardStack.Guard(httptest.NewRequest(http.MethodGet, "/test", nil))

			if (testCase.wantErr != nil && err == nil) || (testCase.wantErr == nil && err != nil) {
				t.Errorf("want error: %v, got: %v", testCase.wantErr, err)
			}

			if testCase.wantErr != nil && err != nil && err.Error() != testCase.wantErr.Error() {
				t.Errorf("want error string: %v, got: %v", testCase.wantErr, err)
			}

			if testCase.wantReq && req == nil {
				t.Fatalf("want request returned, got: nil")
			}

			if !testCase.wantReq && req != nil {
				t.Fatalf("want nil request, got: %v", req)
			}

			if testCase.wantReqCtxVal != "" {
				ctxVal, ok := req.Context().Value(ctxKey{}).(string)
				if !ok {
					t.Errorf("want ctxKey value, got: %v", req.Context().Value(ctxKey{}))
				}

				if testCase.wantReqCtxVal != ctxVal {
					t.Errorf("want request context value: %s, got: %s", testCase.wantReqCtxVal, ctxVal)
				}
			}
		})
	}
}
