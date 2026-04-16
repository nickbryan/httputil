package httputil_test

import (
	"errors"
	"io"
	"log/slog"
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

/*
These tests look like they know too much about the underlying implementation, but that is by design.
We encapsulate the http.Client as a httputil.Client so that we can test our wrapper. We provide
http.Client as the default implementation, allowing us to confidently check that the values
get set as expected rather than having to test the behavioral impact they have on the client itself,
which is already tested within the wrapped http.Client. We just need to know our values are being
set correctly.
*/
func TestClientOptionsDefaults(t *testing.T) {
	t.Parallel()

	const defaultTimeout = time.Minute

	client := httputil.NewClient()
	httpClient := client.Client()

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

	if httpClient.Transport != http.DefaultTransport {
		t.Errorf("expected transport to be http.DefaultTransport, got: %T", httpClient.Transport)
	}
}

func TestClientOptions(t *testing.T) {
	t.Parallel()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("unexpected error creating cookie jar: %v", err)
	}

	spy := &interceptorSpy{}

	client := httputil.NewClient(
		httputil.WithClientBasePath("https://example.com"),
		httputil.WithClientEncoder(&clientTestEncoder{
			contentType: "test/test",
			encode:      func(_ any) (io.Reader, error) { return nil, nil },
		}),
		httputil.WithClientTimeout(10*time.Second),
		httputil.WithClientCookieJar(jar),
		httputil.WithClientRedirectPolicy(func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}),
		httputil.WithClientInterceptor(func(_ http.RoundTripper) http.RoundTripper {
			return spy // Call isn't forwarded on to the next interceptor in the spy.
		}),
	)
	httpClient := client.Client()

	resp, err := client.Get(t.Context(), "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestWithClientTransport(t *testing.T) {
	t.Parallel()

	transportCalled := false

	customTransport := httputil.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		transportCalled = true
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	interceptorCalled := false

	client := httputil.NewClient(
		httputil.WithClientTransport(customTransport),
		httputil.WithClientInterceptor(func(next http.RoundTripper) http.RoundTripper {
			return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				interceptorCalled = true
				return next.RoundTrip(req)
			})
		}),
	)

	resp, err := client.Get(t.Context(), "http://localhost/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestWithClientInterceptor(t *testing.T) {
	t.Parallel()

	t.Run("chains with default transport when none is configured", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		interceptorCalled := false

		client := httputil.NewClient(
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(func(next http.RoundTripper) http.RoundTripper {
				return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					interceptorCalled = true
					return next.RoundTrip(req)
				})
			}),
		)

		resp, err := client.Get(t.Context(), "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		t.Cleanup(func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("closing response body: %s", err)
			}
		})

		if !interceptorCalled {
			t.Error("expected interceptor to be called")
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
		}
	})

	t.Run("interceptors run in the order they are added across calls", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

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
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(makeInterceptor("first")),
			httputil.WithClientInterceptor(makeInterceptor("second")),
		)

		resp, err := client.Get(t.Context(), "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		t.Cleanup(func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("closing response body: %s", err)
			}
		})

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
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(
				makeInterceptor("first"),
				makeInterceptor("second"),
				makeInterceptor("third"),
			),
		)

		resp, err := client.Get(t.Context(), "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		t.Cleanup(func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("closing response body: %s", err)
			}
		})

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

		called := false

		client := httputil.NewClient(
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientInterceptor(nil, func(next http.RoundTripper) http.RoundTripper {
				return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					called = true
					return next.RoundTrip(req)
				})
			}, nil),
		)

		resp, err := client.Get(t.Context(), "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		t.Cleanup(func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("closing response body: %s", err)
			}
		})

		if !called {
			t.Error("expected non-nil interceptor to be called")
		}
	})
}

type clientTestEncoder struct {
	contentType string
	encode      func(data any) (io.Reader, error)
}

func (t *clientTestEncoder) ContentType() string {
	return t.contentType
}

func (t *clientTestEncoder) Encode(data any) (io.Reader, error) {
	return t.encode(data)
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
