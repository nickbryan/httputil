package httputil

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
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

func (s *Server) Serve(ctx context.Context) {
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	errStream := make(chan error, 1)
	go func() {
		defer close(errStream)

		if err := s.Listener.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errStream <- fmt.Errorf("listening for and serving HTTP requests: %w", err)
		}
	}()

	select {
	case err := <-errStream:
		s.logger.ErrorContext(ctx, "Server execution failed", slog.String("error", err.Error()))
		return
	case sig := <-shutdown:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()

		if err := s.Listener.Shutdown(shutdownCtx); err != nil {
			s.logger.ErrorContext(ctx, "Server shutdown failed", slog.String("error", err.Error()))
			return
		}

		<-errStream
		s.logger.InfoContext(ctx, "Server shutdown completed", slog.String("string", sig.String()))
	}
}
