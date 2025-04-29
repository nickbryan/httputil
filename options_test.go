package httputil_test

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/nickbryan/slogutil"

	"github.com/nickbryan/httputil"
)

/*
These tests look like they know too much about the underlying implementation but that is by design.
We encapsulate the http.Server as a httputil.Listener so that we can test our wrapper. We provide
http.Server as the default implementation allowing us to confidently check that the values
get set as expected rather than having to test the behavioral impact they have on the server itself
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
}
