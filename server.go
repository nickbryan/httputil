// Package httputil provides utilities for working with HTTP servers and clients.
package httputil

import (
	"context"
	"errors"
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

// NewServer creates a new Server instance with the specified logger, address, and options.
// The options parameter allows for customization of server settings such as timeouts.
func NewServer(logger *slog.Logger, address string, options ...ServerOption) *Server {
	opts := mapServerOptionsToDefaults(options)

	server := &Server{
		Listener:        nil, // We need to set Listener after we have a server as we pass server as the Handler.
		logger:          logger,
		router:          http.NewServeMux(),
		shutdownTimeout: opts.shutdownTimeout,
		address:         address,
	}

	server.Listener = &http.Server{
		Addr:              server.address,
		Handler:           server,
		ReadTimeout:       opts.readTimeout,
		ReadHeaderTimeout: opts.readHeaderTimeout,
		WriteTimeout:      opts.writeTimeout,
		IdleTimeout:       opts.idleTimeout,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		// TODO: use this or clone the writeHandler so we can provide a static message
		// and add the error as an attribute? Would also allow us to set pc?
	}

	return server
}

// ServeHTTP delegates the request handling to the underlying router. Exposing ServeHTTP
// allows endpoints to be tested without a running server.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Serve starts the HTTP server and listens for incoming requests.
// It gracefully shuts down the server when it receives a SIGINT, SIGTERM, or SIGQUIT signal.
func (s *Server) Serve(ctx context.Context) {
	awaitSignalCtx, cancelAwaitSignal := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		defer cancelAwaitSignal()

		// When Shutdown is called http.ErrServerClosed is returned immediately which unblocks this goroutine.
		if err := s.Listener.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.ErrorContext(ctx, "Server failed to listen and serve", slog.String("error", err.Error()))
		}
	}()

	s.logger.InfoContext(ctx, "Server started", slog.String("address", s.address))
	<-awaitSignalCtx.Done()

	shutdownCtx, cancelShutdown := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancelShutdown()

	// Shutdown blocks while it handles graceful shutdown essentially taking over from ListenAndServe.
	if err := s.Listener.Shutdown(shutdownCtx); err != nil {
		s.logger.ErrorContext(ctx, "Server failed to shutdown gracefully", slog.String("error", err.Error()))
	}

	s.logger.InfoContext(ctx, "Server shutdown")
}
