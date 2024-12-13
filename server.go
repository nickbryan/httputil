// Package httputil provides utilities for working with HTTP servers and clients.
package httputil

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
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
// The options parameter allows for customization of server settings such as the address and timeouts.
func NewServer(logger *slog.Logger, options ...ServerOption) *Server {
	opts := mapServerOptionsToDefaults(options)

	server := &Server{
		Listener:        nil, // We need to set Listener after we have a server as we pass server as the Handler.
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
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		// TODO: use this or clone the writeHandler so we can provide a static message
		// and add the error as an attribute? Would also allow us to set pc?
	}

	return server
}

// Register one or more endpoints with the Server so they are handled by the underlying router.
func (s *Server) Register(endpoints ...Endpoint) {
	for _, endpoint := range endpoints {
		s.router.Handle(endpoint.Method+" "+endpoint.Path, endpoint.Handler)
	}
}

// Serve starts the HTTP server and listens for incoming requests.
// It gracefully shuts down the server when it receives an SIGINT, SIGTERM, or SIGQUIT signal.
func (s *Server) Serve(ctx context.Context) {
	awaitSignalCtx, cancelAwaitSignal := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		defer cancelAwaitSignal()

		if err := s.Listener.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.ErrorContext(ctx, "Server failed to listen and serve", slog.String("error", err.Error()))
		}
	}()

	s.logger.InfoContext(ctx, "Server started", slog.String("address", s.address))
	<-awaitSignalCtx.Done()

	// We use a new context here as inheriting from ctx would create an instant timeout
	// if ctx was canceled. We want to ensure that we still attempt graceful shutdown if this happens.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancelShutdown()

	// Calling Shutdown causes ListenAndServe to return ErrServerClosed immediately. Shutdown then
	// takes over and handles graceful shutdown.
	if err := s.Listener.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // False positive.
		s.logger.ErrorContext(ctx, "Server failed to shutdown gracefully", slog.String("error", err.Error()))
	}

	s.logger.InfoContext(ctx, "Server shutdown")
}

// ServeHTTP delegates the request handling to the underlying router. Exposing ServeHTTP
// allows endpoints to be tested without a running server.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// NewValidator returns a new validator.Validate that is configured for JSON tags.
func NewValidator() *validator.Validate {
	vld := validator.New()

	vld.RegisterTagNameFunc(func(f reflect.StructField) string {
		const tags = 2
		name := strings.SplitN(f.Tag.Get("json"), ",", tags)[0]

		if name == "-" {
			return ""
		}

		return name
	})

	return vld
}
