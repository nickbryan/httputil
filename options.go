package httputil

import (
	"log/slog"
	"time"
)

type (
	// ServerOption allows default server config values to be overridden.
	ServerOption func(so *serverOptions)

	serverOptions struct {
		address           string
		codec             Codec
		idleTimeout       time.Duration
		maxBodySize       int64
		readHeaderTimeout time.Duration
		readTimeout       time.Duration
		shutdownTimeout   time.Duration
		writeTimeout      time.Duration
	}
)

// WithServerAddress sets the address that the Server will listen to and serve on.
func WithServerAddress(address string) ServerOption {
	return func(so *serverOptions) {
		so.address = address
	}
}

// WithServerCodec sets the Codec that the Server will use by default when [NewHandler] is called.
func WithServerCodec(codec Codec) ServerOption {
	return func(so *serverOptions) {
		so.codec = codec
	}
}

// WithServerIdleTimeout sets the idle timeout for the server. This determines how
// long the server will keep an idle connection alive.
func WithServerIdleTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.idleTimeout = timeout
	}
}

// WithServerMaxBodySize sets the maximum allowed size for the request body.
// This limit helps prevent excessive memory usage or abuse from clients
// sending extremely large payloads.
func WithServerMaxBodySize(size int64) ServerOption {
	return func(so *serverOptions) {
		so.maxBodySize = size
	}
}

// WithServerReadHeaderTimeout sets the timeout for reading the request header. This
// is the maximum amount of time the server will wait to receive the request
// headers.
func WithServerReadHeaderTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readHeaderTimeout = timeout
	}
}

// WithServerReadTimeout sets the timeout for reading the request body. This is the
// maximum amount of time the server will wait for the entire request to be
// read.
func WithServerReadTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readTimeout = timeout
	}
}

// WithServerShutdownTimeout sets the timeout for gracefully shutting down the server.
// This is the amount of time the server will wait for existing connections to
// complete before shutting down.
func WithServerShutdownTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.shutdownTimeout = timeout
	}
}

// WithServerWriteTimeout sets the timeout for writing the response. This is the
// maximum amount of time the server will wait to send a response.
func WithServerWriteTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.writeTimeout = timeout
	}
}

// mapServerOptionsToDefaults applies the provided ServerOption to a default
// serverOptions struct.
func mapServerOptionsToDefaults(opts []ServerOption) serverOptions {
	const (
		// 30 seconds is a reasonable balance between resource conservation and keeping
		// connections ready for reuse. It helps prevent excessive resource usage from
		// idle connections while still allowing for HTTP keep-alive benefits.
		defaultIdleTimeout = 30 * time.Second
		// 60 seconds provide sufficient time for clients to upload larger request
		// bodies while preventing malicious or malfunctioning clients from holding
		// connections open indefinitely. This timeout covers the entire request reading
		// process.
		defaultReadTimeout = 60 * time.Second
		// 5MB is a reasonable limit for the maximum body size to protect against
		// excessive memory usage while allowing for fairly large request payloads.
		// This limit helps prevent abuse from clients sending extremely large payloads
		// that could overwhelm the server.
		defaultMaxBodySize = 5 * 1024 * 1024
		// 5 seconds is enough time to receive headers from clients with reasonable
		// network conditions while protecting against slow header attacks where
		// malicious clients send headers very slowly to exhaust server connections.
		defaultReadHeaderTimeout = 5 * time.Second
		// 30 seconds give in-flight requests a reasonable opportunity to complete
		// during server shutdown while ensuring the process doesn't freeze indefinitely
		// if connections don't close properly.
		defaultShutdownTimeout = 30 * time.Second
		// 30 seconds allow for writing responses to slower clients while preventing
		// excessively slow clients from consuming server resources indefinitely. This
		// protects against slow-loris style attacks on the response side.
		defaultWriteTimeout = 30 * time.Second
	)

	defaultOpts := serverOptions{
		address:           ":8080",
		codec:             NewJSONCodec(),
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

type (
	// HandlerOption allows default handler config values to be overridden.
	HandlerOption func(so *handlerOptions)

	handlerOptions struct {
		codec  Codec
		guard  Guard
		logger *slog.Logger
	}
)

// WithHandlerCodec sets the Codec that the Handler will use when [NewHandler] is called.
func WithHandlerCodec(codec Codec) HandlerOption {
	return func(so *handlerOptions) {
		so.codec = codec
	}
}

// WithHandlerGuard sets the Guard that the Handler will use when [NewHandler] is called.
func WithHandlerGuard(guard Guard) HandlerOption {
	return func(so *handlerOptions) {
		so.guard = guard
	}
}

// WithHandlerLogger sets the slog.Logger that the Handler will use when [NewHandler] is called.
func WithHandlerLogger(logger *slog.Logger) HandlerOption {
	return func(so *handlerOptions) {
		so.logger = logger
	}
}

// mapHandlerOptionsToDefaults applies the provided HandlerOption to a default
// handlerOptions struct.
func mapHandlerOptionsToDefaults(opts []HandlerOption) handlerOptions {
	defaultOpts := handlerOptions{
		codec:  nil,
		guard:  nil,
		logger: nil,
	}

	for _, opt := range opts {
		opt(&defaultOpts)
	}

	return defaultOpts
}
