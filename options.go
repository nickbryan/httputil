package httputil

import (
	"time"
)

type (
	// ServerOption allows default config values to be overridden.
	ServerOption func(so *serverOptions)

	serverOptions struct {
		address           string
		idleTimeout       time.Duration
		maxBodySize       int64
		readHeaderTimeout time.Duration
		readTimeout       time.Duration
		shutdownTimeout   time.Duration
		writeTimeout      time.Duration
	}
)

// WithAddress sets the address that the Server will listen and serve on.
func WithAddress(address string) ServerOption {
	return func(so *serverOptions) {
		so.address = address
	}
}

// WithIdleTimeout sets the idle timeout for the server. This determines how
// long the server will keep an idle connection alive.
func WithIdleTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.idleTimeout = timeout
	}
}

// WithMaxBodySize sets the maximum allowed size for the request body.
// This limit helps prevent excessive memory usage or abuse from clients
// sending extremely large payloads.
func WithMaxBodySize(size int64) ServerOption {
	return func(so *serverOptions) {
		so.maxBodySize = size
	}
}

// WithReadHeaderTimeout sets the timeout for reading the request header. This
// is the maximum amount of time the server will wait to receive the request
// headers.
func WithReadHeaderTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readHeaderTimeout = timeout
	}
}

// WithReadTimeout sets the timeout for reading the request body. This is the
// maximum amount of time the server will wait for the entire request to be
// read.
func WithReadTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readTimeout = timeout
	}
}

// WithShutdownTimeout sets the timeout for gracefully shutting down the server.
// This is the amount of time the server will wait for existing connections to
// complete before shutting down.
func WithShutdownTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.shutdownTimeout = timeout
	}
}

// WithWriteTimeout sets the timeout for writing the response. This is the
// maximum amount of time the server will wait to send a response.
func WithWriteTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.writeTimeout = timeout
	}
}

// mapServerOptionsToDefaults applies the provided ServerOptions to a default
// serverOptions struct.
func mapServerOptionsToDefaults(opts []ServerOption) serverOptions {
	const (
		// 30 seconds is a reasonable balance between resource conservation and keeping
		// connections ready for reuse. It helps prevent excessive resource usage from
		// idle connections while still allowing for HTTP keep-alive benefits.
		defaultIdleTimeout = 30 * time.Second
		// 60 seconds provides sufficient time for clients to upload larger request
		// bodies while preventing malicious or malfunctioning clients from holding
		// connections open indefinitely. This timeout covers the entire request reading
		// process.
		defaultReadTimeout = 60 * time.Second
		// 5MB is a reasonable limit for the maximum body size to protect against
		// excessive memory usage while allowing for sufficiently large request payloads.
		// This limit helps prevent abuse from clients sending extremely large payloads
		// that could overwhelm the server.
		defaultMaxBodySize = 5 * 1024 * 1024
		// 5 seconds is enough time to receive headers from clients with reasonable
		// network conditions while protecting against slow header attacks where
		// malicious clients send headers very slowly to exhaust server connections.
		defaultReadHeaderTimeout = 5 * time.Second
		// 30 seconds gives in-flight requests a reasonable opportunity to complete
		// during server shutdown while ensuring the process doesn't hang indefinitely
		// if connections don't close properly.
		defaultShutdownTimeout = 30 * time.Second
		// 30 seconds allows for writing responses to slower clients while preventing
		// excessively slow clients from consuming server resources indefinitely. This
		// protects against slow-loris style attacks on the response side.
		defaultWriteTimeout = 30 * time.Second
	)

	defaultOpts := serverOptions{
		address:           ":8080",
		idleTimeout:       defaultIdleTimeout,
		maxBodySize:       defaultMaxBodySize,
		readHeaderTimeout: defaultReadHeaderTimeout,
		readTimeout:       defaultReadTimeout,
		shutdownTimeout:   defaultShutdownTimeout,
		writeTimeout:      defaultWriteTimeout,
	}

	for _, opt := range opts {
		opt(&defaultOpts)
	}

	return defaultOpts
}
