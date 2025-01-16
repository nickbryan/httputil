package httputil_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/nickbryan/httputil"
)

func TestEndpointsWithMiddleware(t *testing.T) {
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

		endpoints := httputil.EndpointsWithMiddleware(nil)

		if len(endpoints) != 0 {
			t.Errorf("expected len(endpoints) = 0, got: %d", len(endpoints))
		}
	})

	t.Run("returns nothing when no endpoints are passed and middleware is not nil", func(t *testing.T) {
		t.Parallel()

		endpoints := httputil.EndpointsWithMiddleware(injectContextValueMiddleware)

		if len(endpoints) != 0 {
			t.Errorf("expected len(endpoints) = 0, got: %d", len(endpoints))
		}
	})

	t.Run("returns the endpoints when middleware is nil", func(t *testing.T) {
		t.Parallel()

		endpoints := []httputil.Endpoint{
			{Method: http.MethodGet, Path: "/users", Handler: nil},
			{Method: http.MethodPost, Path: "/users", Handler: nil},
		}

		endpointsWithMiddleware := httputil.EndpointsWithMiddleware(nil, endpoints...)

		if len(endpointsWithMiddleware) != len(endpoints) {
			t.Errorf("expected len(endpoints) = %d, got: %d", len(endpoints), len(endpointsWithMiddleware))
		}

		if diff := cmp.Diff(endpoints, endpointsWithMiddleware); diff != "" {
			t.Errorf("returned endpoints are not the same as the passed endpoints, diff: %s", diff)
		}
	})

	t.Run("returns an endpoint with middleware", func(t *testing.T) {
		t.Parallel()

		endpointsWithMiddleware := httputil.EndpointsWithMiddleware(injectContextValueMiddleware, httputil.Endpoint{
			Method:  http.MethodPost,
			Path:    "/users",
			Handler: newTestHandler(t),
		})

		if len(endpointsWithMiddleware) != 1 {
			t.Fatalf("expected len(endpoints) = 1, got: %d", len(endpointsWithMiddleware))
		}

		if !endpointHasMiddleware(endpointsWithMiddleware[0]) {
			t.Error("the handler was not wrapped by the MiddlewareFunc")
		}
	})

	t.Run("returns multiple endpoints with middleware", func(t *testing.T) {
		t.Parallel()

		endpoints := []httputil.Endpoint{
			{Method: http.MethodGet, Path: "/users", Handler: newTestHandler(t)},
			{Method: http.MethodPost, Path: "/users", Handler: newTestHandler(t)},
		}

		endpointsWithMiddleware := httputil.EndpointsWithMiddleware(injectContextValueMiddleware, endpoints...)

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

func TestEndpointsWithPrefix(t *testing.T) {
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

			var endpoints []httputil.Endpoint
			for _, path := range testCase.endpointsPaths {
				endpoints = append(endpoints, httputil.Endpoint{
					Method:  http.MethodGet,
					Path:    path,
					Handler: nil,
				})
			}

			endpoints = httputil.EndpointsWithPrefix(testCase.prefix, endpoints...)

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
