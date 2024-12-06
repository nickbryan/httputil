package httputil

import (
	"time"
)

type (
	// ServerOption allows default config values to be overridden.
	ServerOption func(so *serverOptions)

	serverOptions struct {
		idleTimeout       time.Duration
		readHeaderTimeout time.Duration
		readTimeout       time.Duration
		writeTimeout      time.Duration
		shutdownTimeout   time.Duration
	}
)

// WithIdleTimeout sets the idle timeout for the server.
func WithIdleTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.idleTimeout = timeout
	}
}

// WithReadHeaderTimeout sets the timeout for reading the request header.
func WithReadHeaderTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readHeaderTimeout = timeout
	}
}

// WithReadTimeout sets the timeout for reading the request body.
func WithReadTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readTimeout = timeout
	}
}

// WithShutdownTimeout sets the timeout for gracefully shutting down the server.
func WithShutdownTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.shutdownTimeout = timeout
	}
}

// WithWriteTimeout sets the timeout for writing the response.
func WithWriteTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.writeTimeout = timeout
	}
}

// mapServerOptionsToDefaults applies the provided ServerOptions to a default serverOptions struct.
func mapServerOptionsToDefaults(opts []ServerOption) serverOptions {
	const (
		defaultShutdownTimeout = 5 * time.Second
		defaultTimeout         = time.Minute
	)

	defaultOpts := serverOptions{
		idleTimeout:       defaultTimeout,
		readHeaderTimeout: defaultTimeout,
		readTimeout:       defaultTimeout,
		shutdownTimeout:   defaultShutdownTimeout,
		writeTimeout:      defaultTimeout,
	}

	for _, opt := range opts {
		opt(&defaultOpts)
	}

	return defaultOpts
}
