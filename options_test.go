package httputil_test

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/httputil"
)

func TestClientOptionsDefaults(t *testing.T) {
	t.Parallel()

	const defaultTimeout = time.Minute

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	client := httputil.NewClient(logger)
	httpClient := httputil.ClientHTTPClient(client)

	if client.BasePath() != "" {
		t.Errorf("expected base path to be empty, got: %s", client.BasePath())
	}

	if httpClient.Timeout != defaultTimeout {
		t.Errorf("expected timeout to be %s, got: %s", defaultTimeout, httpClient.Timeout)
	}

	if httpClient.CheckRedirect != nil {
		t.Error("expected redirect check to be nil")
	}

	if httpClient.Jar != nil {
		t.Error("expected cookie jar to be nil")
	}

	if httpClient.Transport == nil {
		t.Errorf("expected transport to be set")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(server.Close)

	defaultClient := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, defaultClient.BasePath()+"/", http.NoBody)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	resp, err := defaultClient.Do(req)
	if err != nil {
		t.Fatalf("executing default client request: %v", err)
	}

	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %s", err)
		}
	})

	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, resp.StatusCode)
	}
}

func TestWithClientTransport(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	transportCalled := false

	customTransport := httputil.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		transportCalled = true
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	interceptorCalled := false

	client := httputil.NewClient(
		logger,
		httputil.WithClientTransport(customTransport),
		httputil.WithClientInterceptor(func(next http.RoundTripper) http.RoundTripper {
			return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				interceptorCalled = true
				return next.RoundTrip(req)
			})
		}),
	)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost/", http.NoBody)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("executing request with custom transport: %v", err)
	}

	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %s", err)
		}
	})

	if !interceptorCalled {
		t.Error("expected interceptor to be called")
	}

	if !transportCalled {
		t.Error("expected custom transport to be called")
	}
}

func TestWithClientTransport_nil_falls_back_to_default(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	client := httputil.NewClient(
		logger,
		httputil.WithClientBasePath(server.URL),
		httputil.WithClientTransport(nil),
	)

	result, err := httputil.Get[struct{}](t.Context(), client, "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
	}
}

func TestClientOptions(t *testing.T) {
	t.Parallel()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("unexpected error creating cookie jar: %v", err)
	}

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	spy := &interceptorSpy{}

	client := httputil.NewClient(
		logger,
		httputil.WithClientBasePath("https://example.com"),
		httputil.WithClientCodec(&fakeClientCodec{contentType: "test/test"}),
		httputil.WithClientTimeout(10*time.Second),
		httputil.WithClientCookieJar(jar),
		httputil.WithClientRedirectPolicy(func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}),
		httputil.WithClientInterceptor(func(_ http.RoundTripper) http.RoundTripper {
			return spy // Call isn't forwarded on to the next interceptor in the spy.
		}),
	)
	httpClient := httputil.ClientHTTPClient(client)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/", http.NoBody)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("executing request with client options: %v", err)
	}

	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %s", err)
		}
	})

	if client.BasePath() != "https://example.com" {
		t.Errorf("expected base path to be https://example.com, got: %s", client.BasePath())
	}

	if !spy.roundtripCalled {
		t.Error("expected roundtrip to be called on the transport")
	}

	if httpClient.Timeout != 10*time.Second {
		t.Errorf("expected timeout to be 10s, got: %s", httpClient.Timeout)
	}

	if httpClient.Jar != jar {
		t.Error("expected cookie jar to be set")
	}

	if httpClient.CheckRedirect == nil {
		t.Error("expected redirect policy to be set")
	}
}

func TestWithClientInterceptor(t *testing.T) {
	t.Parallel()

	t.Run("chains with default transport when none is configured", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

		interceptorCalled := false

		client := httputil.NewClient(
			logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(func(next http.RoundTripper) http.RoundTripper {
				return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					interceptorCalled = true
					return next.RoundTrip(req)
				})
			}),
		)

		result, err := httputil.Get[struct{}](t.Context(), client, "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !interceptorCalled {
			t.Error("expected interceptor to be called")
		}

		if result.StatusCode != http.StatusOK {
			t.Errorf("expected status code %d, got %d", http.StatusOK, result.StatusCode)
		}
	})

	t.Run("interceptors run in the order they are added across calls", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

		var calls []string

		makeInterceptor := func(name string) httputil.InterceptorFunc {
			return func(next http.RoundTripper) http.RoundTripper {
				return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					calls = append(calls, name+"-before")
					resp, err := next.RoundTrip(req)

					calls = append(calls, name+"-after")

					return resp, err
				})
			}
		}

		client := httputil.NewClient(
			logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(makeInterceptor("first")),
			httputil.WithClientInterceptor(makeInterceptor("second")),
		)

		_, err := httputil.Get[struct{}](t.Context(), client, "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"first-before", "second-before", "second-after", "first-after"}
		if !slices.Equal(calls, want) {
			t.Errorf("interceptor order mismatch\nwant: %v\n got: %v", want, calls)
		}
	})

	t.Run("variadic interceptors run in the order they are given", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

		var calls []string

		makeInterceptor := func(name string) httputil.InterceptorFunc {
			return func(next http.RoundTripper) http.RoundTripper {
				return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					calls = append(calls, name+"-before")
					resp, err := next.RoundTrip(req)

					calls = append(calls, name+"-after")

					return resp, err
				})
			}
		}

		client := httputil.NewClient(
			logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(
				makeInterceptor("first"),
				makeInterceptor("second"),
				makeInterceptor("third"),
			),
		)

		_, err := httputil.Get[struct{}](t.Context(), client, "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{
			"first-before", "second-before", "third-before",
			"third-after", "second-after", "first-after",
		}
		if !slices.Equal(calls, want) {
			t.Errorf("interceptor order mismatch\nwant: %v\n got: %v", want, calls)
		}
	})

	t.Run("nil interceptors are skipped", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		called := false

		interceptor := func(next http.RoundTripper) http.RoundTripper {
			return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				called = true
				return next.RoundTrip(req)
			})
		}

		client := httputil.NewClient(
			logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(nil, interceptor, nil),
		)

		_, err := httputil.Get[struct{}](t.Context(), client, "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !called {
			t.Error("expected non-nil interceptor to be called")
		}
	})
}

func TestWithClientTimeout(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := httputil.NewClient(
		logger,
		httputil.WithClientBasePath(server.URL),
		httputil.WithClientTimeout(1*time.Millisecond),
	)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, client.BasePath()+"/", http.NoBody)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	_, err = client.Do(req) //nolint:bodyclose // Error path, no body.
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	netErr, ok := errors.AsType[net.Error](err)
	if !ok || !netErr.Timeout() {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestWithClientRedirectPolicy(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	client := httputil.NewClient(
		logger,
		httputil.WithClientBasePath(server.URL),
		httputil.WithClientRedirectPolicy(func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}),
	)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, client.BasePath()+"/", http.NoBody)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("executing request with redirect policy: %v", err)
	}

	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %s", err)
		}
	})

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected status code %d (redirect not followed), got %d", http.StatusFound, resp.StatusCode)
	}
}

func TestWithClientCookieJar(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	cookieReceived := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			w.WriteHeader(http.StatusOK)

			return
		}

		if cookie, err := r.Cookie("session"); err == nil && cookie.Value == "abc123" {
			cookieReceived = true
		}

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("unexpected error creating cookie jar: %v", err)
	}

	client := httputil.NewClient(
		logger,
		httputil.WithClientBasePath(server.URL),
		httputil.WithClientCookieJar(jar),
	)

	_, err = httputil.Get[struct{}](t.Context(), client, "/set")
	if err != nil {
		t.Fatalf("unexpected error on /set: %v", err)
	}

	_, err = httputil.Get[struct{}](t.Context(), client, "/check")
	if err != nil {
		t.Fatalf("unexpected error on /check: %v", err)
	}

	if !cookieReceived {
		t.Error("expected cookie to be sent on second request")
	}
}

type interceptorSpy struct {
	roundtripCalled bool
}

func (t *interceptorSpy) RoundTrip(_ *http.Request) (*http.Response, error) {
	t.roundtripCalled = true
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

/*
These tests look like they know too much about the underlying implementation, but that is by design.
We encapsulate the http.Server as a httputil.Listener so that we can test our wrapper. We provide
http.Server as the default implementation, allowing us to confidently check that the values
get set as expected rather than having to test the behavioral impact they have on the server itself,
which is already tested within the wrapped http.Server. We just need to know our values are being
set correctly.

Shutdown timeout is tested as part of Server.Serve.
*/
func TestServerOptionsDefaults(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
	server := httputil.NewServer(logger)

	netHTTPServer, ok := server.Listener.(*http.Server)
	if !ok {
		t.Fatal("listener is not a http.Server")
	}

	const (
		defaultIdleTimeout       = 30 * time.Second
		defaultReadTimeout       = 60 * time.Second
		defaultReadHeaderTimeout = 5 * time.Second
		defaultWriteTimeout      = 30 * time.Second
	)

	if got, want := netHTTPServer.Addr, ":8080"; got != want {
		t.Errorf("default address not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.IdleTimeout, defaultIdleTimeout; got != want {
		t.Errorf("default idle timeout not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.ReadHeaderTimeout, defaultReadHeaderTimeout; got != want {
		t.Errorf("default read header timeout not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.ReadTimeout, defaultReadTimeout; got != want {
		t.Errorf("default read timeout not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.WriteTimeout, defaultWriteTimeout; got != want {
		t.Errorf("default write timeout not set, got: %s, want: %s", got, want)
	}
}

// Shutdown timeout is tested as part of Server.Serve.
func TestServerOptions(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
	server := httputil.NewServer(logger,
		httputil.WithServerAddress("someaddr:8765"),
		httputil.WithServerIdleTimeout(time.Duration(1)),
		httputil.WithServerReadHeaderTimeout(time.Duration(2)),
		httputil.WithServerReadTimeout(time.Duration(3)),
		httputil.WithServerWriteTimeout(time.Duration(4)),
		httputil.WithServerCodec(serverTestCodec{}),
	)

	netHTTPServer, ok := server.Listener.(*http.Server)
	if !ok {
		t.Fatal("listener is not a http.Server")
	}

	if got, want := netHTTPServer.Addr, "someaddr:8765"; got != want {
		t.Errorf("default address not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.IdleTimeout, time.Duration(1); got != want {
		t.Errorf("default idle timeout not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.ReadHeaderTimeout, time.Duration(2); got != want {
		t.Errorf("default read header timeout not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.ReadTimeout, time.Duration(3); got != want {
		t.Errorf("default read timeout not set, got: %s, want: %s", got, want)
	}

	if got, want := netHTTPServer.WriteTimeout, time.Duration(4); got != want {
		t.Errorf("default write timeout not set, got: %s, want: %s", got, want)
	}

	server.Register(httputil.Endpoint{
		Method: http.MethodGet,
		Path:   "/",
		Handler: httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
			// Returning data here forces serverTestCodec.Encode to be called, so we know that
			// the global server ServerCodec is overwritten by WithServerCodec during setup.
			return httputil.OK(map[string]any{})
		}),
	})

	res := httptest.NewRecorder()

	server.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	if res.Header().Get("X-Test-Codec") != "true" {
		t.Error("expected X-Test-ServerCodec header to be set by the test codec")
	}
}

func TestHandlerOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithHandlerCodec", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		handler := httputil.NewHandler(
			func(_ httputil.RequestEmpty) (*httputil.Response, error) {
				// Returning data here forces serverTestCodec.Encode to be called, so we know that
				// the global server ServerCodec is overwritten by WithServerCodec during setup.
				return httputil.OK(map[string]any{})
			},
			httputil.WithHandlerCodec(serverTestCodec{}),
			httputil.WithHandlerLogger(logger),
		)

		res := httptest.NewRecorder()

		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

		if res.Header().Get("X-Test-Codec") != "true" {
			t.Error("expected X-Test-ServerCodec header to be set by the test codec")
		}
	})

	t.Run("WithHandlerGuard", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelInfo)
		handler := httputil.NewHandler(
			func(_ httputil.RequestEmpty) (*httputil.Response, error) {
				t.Fatal("action should not be called when guard returns an error")
				return nil, nil
			},
			httputil.WithHandlerCodec(httputil.NewJSONServerCodec()),
			httputil.WithHandlerLogger(logger),
			httputil.WithHandlerGuard(testGuard{}),
		)

		res := httptest.NewRecorder()

		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

		if res.Code != http.StatusInternalServerError {
			t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, res.Code)
		}
	})

	t.Run("WithHandlerLogger", func(t *testing.T) {
		t.Parallel()

		logger, logs := slogutil.NewInMemoryLogger(slog.LevelInfo)
		expectedErr := errors.New("unhandled action error")

		handler := httputil.NewHandler(
			func(_ httputil.RequestEmpty) (*httputil.Response, error) {
				return nil, expectedErr
			},
			httputil.WithHandlerCodec(httputil.NewJSONServerCodec()),
			httputil.WithHandlerLogger(logger),
		)

		res := httptest.NewRecorder()

		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

		if res.Code != http.StatusInternalServerError {
			t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, res.Code)
		}

		query := slogmem.RecordQuery{
			Message: "Handler received an unhandled error",
			Level:   slog.LevelError,
			Attrs: map[string]slog.Value{
				"error": slog.AnyValue("calling action: unhandled action error"),
			},
		}

		if ok, diff := logs.Contains(query); !ok {
			t.Errorf("expected log record not found, diff (-want +got):\n%s", diff)
		}
	})
}

type (
	serverTestCodec struct {
		httputil.ServerCodec
	}
	testGuard struct{}
)

func (t serverTestCodec) Encode(w http.ResponseWriter, _ int, _ any) error {
	w.Header().Set("X-Test-Codec", "true")
	return nil
}

func (g testGuard) Guard(_ *http.Request) (*http.Request, error) {
	return nil, errors.New("access denied")
}
