package httputil

import (
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

	address         string
	shutdownTimeout time.Duration
}

func NewServer(logger *slog.Logger, address string, options ...ServerOption) *Server {
	opts := mapServerOptionsToDefaults(options)

	server := &Server{
		Listener:        nil,
		logger:          logger,
		router:          http.NewServeMux(),
		shutdownTimeout: opts.shutdownTimeout,
		address:         address,
	}

	// TODO: leave like this or don't be exhaustive?
	server.Listener = &http.Server{
		Addr:                         server.address,
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
	awaitSignalCtx, cancelSignalCtx := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		defer cancelSignalCtx()

		// When Listener.Shutdown is called http.ErrServerClosed is returned immediately unblocking this
		// goroutine. Shutdown then blocks while it handles graceful shutdown.
		if err := s.Listener.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.ErrorContext(ctx, "Server failed to listen and serve", slog.String("error", err.Error()))
		}
	}()

	s.logger.InfoContext(ctx, "Server started", slog.String("address", s.address))
	<-awaitSignalCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()

	if err := s.Listener.Shutdown(shutdownCtx); err != nil {
		s.logger.ErrorContext(ctx, "Server failed to shutdown gracefully", slog.String("error", err.Error()))
	}

	s.logger.InfoContext(ctx, "Server shutdown completed")
}
