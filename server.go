package httputil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var ErrSignalChannelClosed = errors.New("signal channel has been closed unexpectedly")

type SignalInterruptError struct {
	Signal os.Signal
}

func (e *SignalInterruptError) Error() string {
	return fmt.Sprintf("received interrupt signal: %s", e.Signal)
}

type Server struct {
	Listener interface {
		ListenAndServe() error
		Shutdown(ctx context.Context) error
	}

	logger *slog.Logger
	router *http.ServeMux

	shutdownTimeout time.Duration
}

func NewServer(logger *slog.Logger, address string, options ...ServerOption) *Server {
	opts := mapServerOptionsToDefaults(options)

	server := &Server{
		Listener:        nil,
		logger:          logger,
		router:          http.NewServeMux(),
		shutdownTimeout: opts.shutdownTimeout,
	}

	// TODO: leave like this or don't be exhaustive?
	server.Listener = &http.Server{
		Addr:                         address,
		Handler:                      server,
		DisableGeneralOptionsHandler: false,
		TLSConfig:                    nil,
		ReadTimeout:                  opts.readTimeout,
		ReadHeaderTimeout:            opts.readHeaderTimeout,
		WriteTimeout:                 opts.writeTimeout,
		IdleTimeout:                  opts.idleTimeout,
		MaxHeaderBytes:               http.DefaultMaxHeaderBytes,
		TLSNextProto:                 nil,
		ConnState:                    nil,
		// TODO: use this or clone the writeHandler so we can provide a static message and add the error as an attribute? Would also allow us to set pc?
		ErrorLog:    slog.NewLogLogger(logger.Handler(), slog.LevelError),
		BaseContext: nil,
		ConnContext: nil,
	}

	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) Serve(ctx context.Context) error {
	errChan := make(chan error, 1)

	go func(errChan chan error) {
		defer close(errChan)

		if err := s.Listener.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("listening for and serving HTTP requests: %w", err)
		}
	}(errChan)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	select {
	case <-ctx.Done():
		if err := s.shutdownGracefully(context.Background()); err != nil {
			return fmt.Errorf("ctx.Done: %w", err)
		}

		return nil
	case sig, ok := <-sigChan:
		if err := s.shutdownGracefully(context.Background()); err != nil {
			return fmt.Errorf("signal received: %w", err)
		}

		if !ok {
			return ErrSignalChannelClosed
		}

		return &SignalInterruptError{Signal: sig}
	case err, ok := <-errChan:
		if !ok {
			return nil
		}

		return err
	}
}

func (s *Server) shutdownGracefully(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()

	if err := s.Listener.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("ctx.Done, shutting down the server: %w", err)
	}

	return nil
}
