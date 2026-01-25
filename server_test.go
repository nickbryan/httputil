package httputil_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"

	"github.com/nickbryan/httputil"
)

//nolint:paralleltest // These test do not run in parallel due to how signal notifications are handled and tested.
func TestServer_Serve(t *testing.T) {
	const testAddress = "test:address"

	var (
		startedLog = slogmem.RecordQuery{
			Level:   slog.LevelInfo,
			Message: "Server started",
			Attrs:   map[string]slog.Value{"address": slog.StringValue(testAddress)},
		}
		shutdownLog = slogmem.RecordQuery{
			Level:   slog.LevelInfo,
			Message: "Server shutdown",
			Attrs:   nil,
		}
	)

	testCases := map[string]struct {
		ctxFactory           func() context.Context
		signal               os.Signal
		listenAndServeErr    error
		shutdownErr          error
		simulateLongShutdown bool
		wantLogs             []slogmem.RecordQuery
	}{
		"shuts down successfully after receiving SIGINT": {
			ctxFactory:           t.Context,
			signal:               syscall.SIGINT,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs:             []slogmem.RecordQuery{startedLog, shutdownLog},
		},
		"shuts down successfully after receiving SIGTERM": {
			ctxFactory:           t.Context,
			signal:               syscall.SIGTERM,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs:             []slogmem.RecordQuery{startedLog, shutdownLog},
		},
		"shuts down successfully after receiving SIGQUIT": {
			ctxFactory:           t.Context,
			signal:               syscall.SIGQUIT,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs:             []slogmem.RecordQuery{startedLog, shutdownLog},
		},
		"shuts down successfully if the context is canceled": {
			ctxFactory: func() context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx
			},
			signal:               nil,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs:             []slogmem.RecordQuery{startedLog, shutdownLog},
		},
		"shuts down with an error log if listening and serving fails": {
			ctxFactory:           t.Context,
			signal:               nil,
			listenAndServeErr:    errors.New("listen and serve error"),
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs: []slogmem.RecordQuery{startedLog, {
				Level:   slog.LevelError,
				Message: "Server failed to listen and serve",
				Attrs:   map[string]slog.Value{"error": slog.StringValue("listen and serve error")},
			}, shutdownLog},
		},
		"shuts down with an error log if shutdown returns an error": {
			ctxFactory:           t.Context,
			signal:               syscall.SIGINT,
			listenAndServeErr:    nil,
			shutdownErr:          errors.New("shutdown error"),
			simulateLongShutdown: false,
			wantLogs: []slogmem.RecordQuery{startedLog, {
				Level:   slog.LevelError,
				Message: "Server failed to shutdown gracefully",
				Attrs:   map[string]slog.Value{"error": slog.StringValue("shutdown error")},
			}, shutdownLog},
		},
		"shut down times out if deadline is exceeded": {
			ctxFactory:           t.Context,
			signal:               syscall.SIGINT,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: true,
			wantLogs: []slogmem.RecordQuery{startedLog, {
				Level:   slog.LevelError,
				Message: "Server failed to shutdown gracefully",
				Attrs:   map[string]slog.Value{"error": slog.StringValue("context deadline exceeded")},
			}, shutdownLog},
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			const shutdownTimeout = 50 * time.Millisecond

			connCloseDuration := shutdownTimeout / 2
			if testCase.simulateLongShutdown {
				connCloseDuration = shutdownTimeout * 2
			}

			logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
			server := httputil.NewServer(logger, httputil.WithServerAddress(testAddress), httputil.WithServerShutdownTimeout(shutdownTimeout))

			server.Listener = &fakeListener{
				listenAndServeErr: testCase.listenAndServeErr,
				shutdownErr:       testCase.shutdownErr,
				connCloseDuration: connCloseDuration,
				listenChan:        make(chan any),
			}

			if testCase.signal != nil {
				if err := sendFutureSignalNotification(t.Context(), t, testCase.signal); err != nil {
					t.Fatalf("unexpected error sending signal notification: %s", err.Error())
				}
			}

			server.Serve(testCase.ctxFactory())

			if logs.Len() != len(testCase.wantLogs) {
				fmtLogs, err := json.MarshalIndent(logs.AsSliceOfNestedKeyValuePairs(), "", " ")
				if err != nil {
					t.Fatalf("unexpected error marshaling logged records, err: %s, records: %+v", err.Error(), logs.AsSliceOfNestedKeyValuePairs())
				}

				t.Errorf("unexpected number of logs produced, want: %d, got: %d\n%s", len(testCase.wantLogs), logs.Len(), fmtLogs)
			}

			for _, query := range testCase.wantLogs {
				if ok, diff := logs.Contains(query); !ok {
					t.Errorf("logs does not contain query, want: %+v, got:\n%s", query, diff)
				}
			}
		})
	}
}

func TestServer_ServeHTTP(t *testing.T) {
	t.Parallel()

	t.Run("recovers from a panic gracefully", func(t *testing.T) {
		t.Parallel()

		logger, records := slogutil.NewInMemoryLogger(slog.LevelDebug)
		svr := httputil.NewServer(logger)

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)

		svr.Register(httputil.Endpoint{
			Method: http.MethodGet,
			Path:   "/",
			Handler: httputil.WrapNetHTTPHandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				panic("panic from handler")
			}),
		})

		svr.ServeHTTP(response, request)

		if response.Result().StatusCode != http.StatusInternalServerError {
			t.Errorf("unexpected status code, want: %d, got: %d", http.StatusInternalServerError, response.Result().StatusCode)
		}

		want := []map[string]any{{
			slog.LevelKey:   slog.LevelError,
			slog.MessageKey: "Handler panicked",
			"error":         "panic from handler",
		}}

		if diff := cmp.Diff(want, records.AsSliceOfNestedKeyValuePairs(), cmpopts.IgnoreMapEntries(func(k string, _ any) bool {
			return k == slog.TimeKey || k == "stack" // Stack trace will be different on each machine so ignore its output.
		})); diff != "" {
			t.Errorf("logs does not contain query, want: %+v, got:\n%s", want, diff)
		}
	})

	t.Run("limits the request body", func(t *testing.T) {
		t.Parallel()

		logger, records := slogutil.NewInMemoryLogger(slog.LevelDebug)
		svr := httputil.NewServer(logger, httputil.WithServerMaxBodySize(0))

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("some request body"))

		svr.Register(httputil.Endpoint{
			Method: http.MethodPost,
			Path:   "/",
			Handler: httputil.WrapNetHTTPHandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if _, err := io.ReadAll(r.Body); err != nil {
					t.Fatalf("unexpected error reading request body: %s", err.Error())
				}
			}),
		})

		svr.ServeHTTP(response, request)

		if response.Result().StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("unexpected status code, want: %d, got: %d", http.StatusRequestEntityTooLarge, response.Result().StatusCode)
		}

		query := slogmem.RecordQuery{
			Level:   slog.LevelWarn,
			Message: "Request body exceeds max bytes limit",
			Attrs: map[string]slog.Value{
				"max_bytes": slog.Int64Value(0),
			},
		}

		if ok, diff := records.Contains(query); !ok {
			t.Errorf("logs does not contain query, diff:\n%s", diff)
		}
	})
}

func TestNetHTTPServerLogAdapter(t *testing.T) {
	t.Parallel()

	logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
	server := httputil.NewServer(logger)

	netHTTPServer, ok := server.Listener.(*http.Server)
	if !ok {
		t.Fatalf("listener is not a http.Server")
	}

	netHTTPServer.ErrorLog.Print("some internal server error message")

	if logs.Len() != 1 {
		t.Errorf("unexpected number of logs produced, want: 1, got: %d", logs.Len())
	}

	query := slogmem.RecordQuery{
		Level:   slog.LevelError,
		Message: "Internal error logged by net/http server",
		Attrs: map[string]slog.Value{
			"error": slog.AnyValue("some internal server error message"),
		},
	}

	if ok, diff := logs.Contains(query); !ok {
		t.Errorf("logs does not contain query, want: %+v, got:\n%s", query, diff)
	}
}

func sendFutureSignalNotification(ctx context.Context, t *testing.T, sig os.Signal) (returnErr error) {
	t.Helper()

	// Give the server code time to start before sending the signal.
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	t.Cleanup(cancel)

	go func() {
		<-ctx.Done()

		proc, err := os.FindProcess(os.Getpid())
		if err != nil {
			returnErr = fmt.Errorf("finding process: %w", err)
			return
		}

		err = proc.Signal(sig)
		if err != nil {
			returnErr = fmt.Errorf("sending signal to process: %w", err)
		}
	}()

	return returnErr
}

type fakeListener struct {
	listenChan        chan any
	connCloseDuration time.Duration
	listenAndServeErr error
	shutdownErr       error
}

func (fl *fakeListener) ListenAndServe() error {
	if fl.listenAndServeErr != nil {
		return fl.listenAndServeErr
	}

	// Simulate the server blocking to receive and handle connections.
	<-fl.listenChan

	// This is the behavior of net/http Server.ListenAndServe when Shutdown is called.
	return http.ErrServerClosed
}

func (fl *fakeListener) Shutdown(ctx context.Context) error {
	// Stop blocking ListenAndServe to allow the goroutine to exit.
	close(fl.listenChan)

	simulateConnCloseCtx, cancelSimulateConnClose := context.WithTimeout(context.Background(), fl.connCloseDuration)
	defer cancelSimulateConnClose()

	select {
	case <-simulateConnCloseCtx.Done():
		return fl.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
