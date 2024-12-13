package httputil_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"
)

//nolint:paralleltest // These test do not run in parallel due to how signal notifications are handled and tested.
func TestServerServe(t *testing.T) {
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
			ctxFactory:           context.Background,
			signal:               syscall.SIGINT,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs:             []slogmem.RecordQuery{startedLog, shutdownLog},
		},
		"shuts down successfully after receiving SIGTERM": {
			ctxFactory:           context.Background,
			signal:               syscall.SIGTERM,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs:             []slogmem.RecordQuery{startedLog, shutdownLog},
		},
		"shuts down successfully after receiving SIGQUIT": {
			ctxFactory:           context.Background,
			signal:               syscall.SIGQUIT,
			listenAndServeErr:    nil,
			shutdownErr:          nil,
			simulateLongShutdown: false,
			wantLogs:             []slogmem.RecordQuery{startedLog, shutdownLog},
		},
		"shuts down successfully if the context is canceled": {
			ctxFactory: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
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
			ctxFactory:           context.Background,
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
			ctxFactory:           context.Background,
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
		"shut down times out if deadline is exceded": {
			ctxFactory:           context.Background,
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
			server := httputil.NewServer(logger, httputil.WithAddress(testAddress), httputil.WithShutdownTimeout(shutdownTimeout))

			server.Listener = &fakeListener{
				listenAndServeErr: testCase.listenAndServeErr,
				shutdownErr:       testCase.shutdownErr,
				connCloseDuration: connCloseDuration,
				listenChan:        make(chan any),
			}

			if testCase.signal != nil {
				if err := sendFutureSignalNotification(context.Background(), t, testCase.signal); err != nil {
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
		return ctx.Err() //nolint:wrapcheck // We just want the underlying error here for the test.
	}
}
