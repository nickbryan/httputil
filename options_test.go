package httputil_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/httputil"
)

/*
These tests look like they know too much about the underlying implementation, but that is by design.
We encapsulate the http.Server as a httputil.Listener so that we can test our wrapper. We provide
http.Server as the default implementation, allowing us to confidently check that the values
get set as expected rather than having to test the behavioral impact they have on the server itself,
which is already tested within the wrapped http.Server. We just need to know our values are being
set correctly.
*/

// Shutdown timeout is tested as part of Server.Serve.
func TestServerOptionsDefaults(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
	server := httputil.NewServer(logger)

	netHTTPServer, ok := server.Listener.(*http.Server)
	if !ok {
		t.Fatalf("listener is not a http.Server")
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
		t.Fatalf("listener is not a http.Server")
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
		t.Errorf("expected X-Test-ServerCodec header to be set by the test codec")
	}
}

func TestHandlerOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithHandlerCodec", func(t *testing.T) {
		t.Parallel()

		handler := httputil.NewHandler(
			func(_ httputil.RequestEmpty) (*httputil.Response, error) {
				// Returning data here forces serverTestCodec.Encode to be called, so we know that
				// the global server ServerCodec is overwritten by WithServerCodec during setup.
				return httputil.OK(map[string]any{})
			},
			httputil.WithHandlerCodec(serverTestCodec{}),
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
			func(r httputil.RequestEmpty) (*httputil.Response, error) {
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
			func(r httputil.RequestEmpty) (*httputil.Response, error) {
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

func (t serverTestCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("X-Test-Codec", "true")
	return nil
}

func (g testGuard) Guard(_ *http.Request) (*http.Request, error) {
	return nil, errors.New("access denied")
}

type clientTestCodec struct {
	t           *testing.T
	contentType string
	encode      func(data any) (io.Reader, error)
	decode      func(r io.Reader, into any) error
}

func (t *clientTestCodec) ContentType() string {
	return t.contentType
}

func (t *clientTestCodec) Encode(data any) (io.Reader, error) {
	return t.encode(data)
}

func (t *clientTestCodec) Decode(r io.Reader, into any) error {
	return t.decode(r, into)
}
