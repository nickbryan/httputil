package httputil_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/internal/testutil"
	"github.com/nickbryan/httputil/problem"
)

// opaqueMiddleware wraps a handler in a new http.Handler, hiding the inner
// handler from type assertions. This simulates standard third-party middleware
// that would have broken the old setter-based dependency injection.
func opaqueMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func TestHandlerContextResolution(t *testing.T) {
	t.Parallel()

	t.Run("handler wrapped in opaque middleware resolves deps", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodGet,
			Path:   "/test",
			Handler: opaqueMiddleware(httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
				return httputil.OK(map[string]string{"ok": "true"})
			})),
		})

		res := httptest.NewRecorder()
		server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/test", nil))

		if res.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", res.Code, http.StatusOK)
		}

		if diff := testutil.DiffJSON(`{"ok":"true"}`, res.Body.String()); diff != "" {
			t.Errorf("body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("handler with WithHandlerCodec wrapped in opaque middleware uses handler codec", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.Endpoint{
			Method: http.MethodGet,
			Path:   "/test",
			Handler: opaqueMiddleware(httputil.NewHandler(
				func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.OK(map[string]any{})
				},
				httputil.WithHandlerCodec(serverTestCodec{}),
			)),
		})

		res := httptest.NewRecorder()
		server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/test", nil))

		if res.Header().Get("X-Test-Codec") != "true" {
			t.Error("expected handler-level codec to be used, not server codec")
		}
	})

	t.Run("handler with WithHandlerGuard wrapped in opaque middleware uses handler guard", func(t *testing.T) {
		t.Parallel()

		handlerGuardCalled := false
		endpointGuardCalled := false

		handlerGuard := httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
			handlerGuardCalled = true
			return r, nil
		})
		endpointGuard := httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
			endpointGuardCalled = true
			return r, nil
		})

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(httputil.NewEndpointWithGuard(httputil.Endpoint{
			Method: http.MethodGet,
			Path:   "/test",
			Handler: opaqueMiddleware(httputil.NewHandler(
				func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return httputil.NoContent()
				},
				httputil.WithHandlerGuard(handlerGuard),
			)),
		}, endpointGuard))

		res := httptest.NewRecorder()
		server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/test", nil))

		if !handlerGuardCalled {
			t.Error("expected handler-level guard to be called")
		}

		if endpointGuardCalled {
			t.Error("expected endpoint-level guard NOT to be called when handler guard is set")
		}
	})

	t.Run("handler with WithHandlerLogger wrapped in opaque middleware uses handler logger", func(t *testing.T) {
		t.Parallel()

		serverLogger, serverLogs := slogutil.NewInMemoryLogger(slog.LevelDebug)
		handlerLogger, handlerLogs := slogutil.NewInMemoryLogger(slog.LevelDebug)

		server := httputil.NewServer(serverLogger)

		server.Register(httputil.Endpoint{
			Method: http.MethodGet,
			Path:   "/test",
			Handler: opaqueMiddleware(httputil.NewHandler(
				func(_ httputil.RequestEmpty) (*httputil.Response, error) {
					return nil, errors.New("trigger logging")
				},
				httputil.WithHandlerCodec(httputil.NewJSONServerCodec()),
				httputil.WithHandlerLogger(handlerLogger),
			)),
		})

		res := httptest.NewRecorder()
		server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/test", nil))

		query := slogmem.RecordQuery{
			Message: "Handler received an unhandled error",
			Level:   slog.LevelError,
		}

		if ok, _ := handlerLogs.Contains(query); !ok {
			t.Error("expected log entry on handler logger")
		}

		if ok, _ := serverLogs.Contains(query); ok {
			t.Error("expected no log entry on server logger for handler-level logged error")
		}
	})

	t.Run("handler with guard wrapped in opaque middleware applies guard", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(
			httputil.EndpointGroup{
				httputil.NewEndpointWithGuard(httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: opaqueMiddleware(httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
						t.Fatal("action should not be called when guard returns an error")
						return nil, nil
					})),
				}, errorGuard{}),
			}...,
		)

		res := httptest.NewRecorder()
		server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/test", nil))

		if res.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", res.Code, http.StatusInternalServerError)
		}

		if diff := testutil.DiffJSON(problem.ServerError(httptest.NewRequest(http.MethodGet, "/test", http.NoBody)).MustMarshalJSONString(), res.Body.String()); diff != "" {
			t.Errorf("body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("netHTTPHandler wrapped in opaque middleware resolves logger", func(t *testing.T) {
		t.Parallel()

		logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(
			httputil.EndpointGroup{
				httputil.NewEndpointWithGuard(httputil.Endpoint{
					Method: http.MethodGet,
					Path:   "/test",
					Handler: opaqueMiddleware(httputil.WrapNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusNoContent)
					})),
				}, errorGuard{}),
			}...,
		)

		res := httptest.NewRecorder()
		server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/test", nil))

		if res.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", res.Code, http.StatusInternalServerError)
		}

		query := slogmem.RecordQuery{
			Message: "Unhandled error received by net/http handler",
			Level:   slog.LevelError,
		}

		if ok, diff := logs.Contains(query); !ok {
			t.Errorf("expected guard error to be logged:\n%s", diff)
		}
	})

	t.Run("handler panics when served without Server due to missing codec", func(t *testing.T) {
		t.Parallel()

		handler := httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
			return httputil.NoContent()
		})

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic")
			}

			want := "httputil: handler *httputil.handler[struct {},struct {}] served without being registered on a Server (missing codec)"
			if r != want {
				t.Errorf("panic message = %q, want %q", r, want)
			}
		}()

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})

	t.Run("handler panics when served without Server due to missing logger", func(t *testing.T) {
		t.Parallel()

		handler := httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
			return httputil.NoContent()
		}, httputil.WithHandlerCodec(httputil.NewJSONServerCodec()))

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic")
			}

			want := "httputil: handler *httputil.handler[struct {},struct {}] served without being registered on a Server (missing logger)"
			if r != want {
				t.Errorf("panic message = %q, want %q", r, want)
			}
		}()

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})

	t.Run("netHTTPHandler panics when served without Server due to missing logger", func(t *testing.T) {
		t.Parallel()

		handler := httputil.WrapNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic")
			}

			want := "httputil: handler *httputil.netHTTPHandler served without being registered on a Server (missing logger)"
			if r != want {
				t.Errorf("panic message = %q, want %q", r, want)
			}
		}()

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})

	t.Run("shared handler with different guards per endpoint uses the correct guard", func(t *testing.T) {
		t.Parallel()

		handler := httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
			ctxVal, ok := r.Context().Value(addToContextGuardCtxKey{}).(addToContextGuard)
			if !ok {
				return httputil.OK(map[string]string{"guard": "none"})
			}

			return httputil.OK(map[string]string{"guard": string(ctxVal)})
		})

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(
			httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet, Path: "/a", Handler: handler,
			}, addToContextGuard("guard-a")),
			httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet, Path: "/b", Handler: handler,
			}, addToContextGuard("guard-b")),
		)

		// Request to /a should use guard-a.
		resA := httptest.NewRecorder()
		server.ServeHTTP(resA, httptest.NewRequest(http.MethodGet, "/a", nil))

		if diff := testutil.DiffJSON(`{"guard":"guard-a"}`, resA.Body.String()); diff != "" {
			t.Errorf("/a body mismatch (-want +got):\n%s", diff)
		}

		// Request to /b should use guard-b.
		resB := httptest.NewRecorder()
		server.ServeHTTP(resB, httptest.NewRequest(http.MethodGet, "/b", nil))

		if diff := testutil.DiffJSON(`{"guard":"guard-b"}`, resB.Body.String()); diff != "" {
			t.Errorf("/b body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("shared netHTTPHandler with different guards per endpoint uses the correct guard", func(t *testing.T) {
		t.Parallel()

		handler := httputil.WrapNetHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctxVal, ok := r.Context().Value(addToContextGuardCtxKey{}).(addToContextGuard)
			if !ok {
				_, _ = w.Write([]byte("none"))
				return
			}

			_, _ = w.Write([]byte(string(ctxVal)))
		})

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		server := httputil.NewServer(logger)

		server.Register(
			httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet, Path: "/a", Handler: handler,
			}, addToContextGuard("guard-a")),
			httputil.NewEndpointWithGuard(httputil.Endpoint{
				Method: http.MethodGet, Path: "/b", Handler: handler,
			}, addToContextGuard("guard-b")),
		)

		resA := httptest.NewRecorder()
		server.ServeHTTP(resA, httptest.NewRequest(http.MethodGet, "/a", nil))

		if resA.Body.String() != "guard-a" {
			t.Errorf("/a body = %q, want %q", resA.Body.String(), "guard-a")
		}

		resB := httptest.NewRecorder()
		server.ServeHTTP(resB, httptest.NewRequest(http.MethodGet, "/b", nil))

		if resB.Body.String() != "guard-b" {
			t.Errorf("/b body = %q, want %q", resB.Body.String(), "guard-b")
		}
	})
}
