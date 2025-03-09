// Package httputil provides utilities for working with HTTP servers and clients.
package httputil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// Server is an HTTP server with graceful shutdown capabilities.
type Server struct {
	// Listener is implemented by a *http.Server, the interface allows us to test Serve.
	Listener interface {
		ListenAndServe() error
		Shutdown(ctx context.Context) error
	}

	logger *slog.Logger
	router *http.ServeMux

	address         string
	shutdownTimeout time.Duration
}

// NewServer creates a new Server instance with the specified logger and options.
// The options parameter allows for customization of server settings such as the
// address and timeouts.
func NewServer(logger *slog.Logger, options ...ServerOption) *Server {
	opts := mapServerOptionsToDefaults(options)

	server := &Server{
		Listener:        nil, // We need to set Listener after we have a server as we pass server as the handler.
		logger:          logger,
		router:          http.NewServeMux(),
		shutdownTimeout: opts.shutdownTimeout,
		address:         opts.address,
	}

	//nolint:exhaustruct // Accept defaults for fields we do not set.
	server.Listener = &http.Server{
		Addr:              server.address,
		Handler:           server,
		ReadTimeout:       opts.readTimeout,
		ReadHeaderTimeout: opts.readHeaderTimeout,
		WriteTimeout:      opts.writeTimeout,
		IdleTimeout:       opts.idleTimeout,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(netHTTPServerLogAdapter{Handler: logger.Handler()}, slog.LevelError),
	}

	return server
}

// Register one or more endpoints with the Server so they are handled by the
// underlying router.
func (s *Server) Register(endpoints ...Endpoint) {
	for _, endpoint := range endpoints {
		endpoint.Handler.use(s.logger, endpoint.guard)
		s.router.Handle(endpoint.Method+" "+endpoint.Path, endpoint.Handler)
	}
}

// Serve starts the HTTP server and listens for incoming requests. It gracefully
// shuts down the server when it receives an SIGINT, SIGTERM, or SIGQUIT signal.
func (s *Server) Serve(ctx context.Context) {
	awaitSignalCtx, cancelAwaitSignal := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		defer cancelAwaitSignal()

		if err := s.Listener.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.ErrorContext(ctx, "Server failed to listen and serve", slog.Any("error", err))
		}
	}()

	s.logger.InfoContext(ctx, "Server started", slog.String("address", s.address))
	<-awaitSignalCtx.Done()

	// We use a new context here as inheriting from ctx would create an instant
	// timeout if ctx was canceled. We want to ensure that we still attempt graceful
	// shutdown if this happens.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancelShutdown()

	// Calling Shutdown causes ListenAndServe to return ErrServerClosed immediately.
	// Shutdown then takes over and handles graceful shutdown.
	if err := s.Listener.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // False positive.
		s.logger.ErrorContext(ctx, "Server failed to shutdown gracefully", slog.Any("error", err))
	}

	s.logger.InfoContext(ctx, "Server shutdown")
}

// ServeHTTP delegates the request handling to the underlying router. Exposing
// ServeHTTP allows endpoints to be tested without a running server.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	newPanicRecoveryMiddleware(s.logger)(s.router).ServeHTTP(w, r)
}

type netHTTPServerLogAdapter struct {
	slog.Handler
}

func (n netHTTPServerLogAdapter) Handle(ctx context.Context, record slog.Record) error {
	rec := record.Clone()

	rec.Message = "Internal error logged by net/http server"
	rec.AddAttrs(slog.Any("error", record.Message))

	if err := n.Handler.Handle(ctx, rec); err != nil {
		return fmt.Errorf("calling inner handler from netHTTPServerLogAdapter: %w", err)
	}

	return nil
}
