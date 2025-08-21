package httputil

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

type (
	// ClientOption allows default doer config values to be overridden.
	ClientOption func(co *clientOptions)

	RedirectPolicy func(req *http.Request, via []*http.Request) error

	clientOptions struct {
		basePath      string
		checkRedirect RedirectPolicy
		codec         ClientCodec
		jar           http.CookieJar
		timeout       time.Duration
	}
)

// WithClientBasePath sets the base path for the Client. This is used to prefix
// relative paths in the request URLs.
func WithClientBasePath(basePath string) ClientOption {
	return func(co *clientOptions) {
		co.basePath = basePath
	}
}

// WithClientCodec sets the ClientCodec that the Client will use when making requests.
func WithClientCodec(codec ClientCodec) ClientOption {
	return func(co *clientOptions) {
		co.codec = codec
	}
}

// WithClientCookieJar sets the CookieJar that the Client will use when making requests.
func WithClientCookieJar(jar http.CookieJar) ClientOption {
	return func(co *clientOptions) {
		co.jar = jar
	}
}

// WithClientTimeout sets the timeout for the doer. This is the maximum amount of
// time the doer will wait for a response from the server.
func WithClientTimeout(timeout time.Duration) ClientOption {
	return func(co *clientOptions) {
		co.timeout = timeout
	}
}

// WithClientRedirectPolicy sets the RedirectPolicy that the Client will use when
// following redirects.
func WithClientRedirectPolicy(policy RedirectPolicy) ClientOption {
	return func(co *clientOptions) {
		co.checkRedirect = policy
	}
}

func mapClientOptionsToDefaults(opts []ClientOption) clientOptions {
	const (
		// This value aligns with the server's read timeout, providing a reasonable
		// balance between waiting for slow server responses and preventing the doer
		// from being stuck for too long
		defaultTimeout = 60 * time.Second
	)

	defaultOpts := clientOptions{
		basePath:      "",
		checkRedirect: nil,
		codec:         NewJSONClientCodec(),
		jar:           nil,
		timeout:       defaultTimeout,
	}

	for _, opt := range opts {
		opt(&defaultOpts)
	}

	return defaultOpts
}

type (
	// HandlerOption allows default handler config values to be overridden.
	HandlerOption func(ho *handlerOptions)

	handlerOptions struct {
		codec  ServerCodec
		guard  Guard
		logger *slog.Logger
	}
)

// WithHandlerCodec sets the ServerCodec that the Handler will use when [NewHandler] is called.
func WithHandlerCodec(codec ServerCodec) HandlerOption {
	return func(ho *handlerOptions) {
		ho.codec = codec
	}
}

// WithHandlerGuard sets the Guard that the Handler will use when [NewHandler] is called.
func WithHandlerGuard(guard Guard) HandlerOption {
	return func(ho *handlerOptions) {
		ho.guard = guard
	}
}

// WithHandlerLogger sets the slog.Logger that the Handler will use when [NewHandler] is called.
func WithHandlerLogger(logger *slog.Logger) HandlerOption {
	return func(ho *handlerOptions) {
		ho.logger = logger
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

type (
	// RequestOption allows default request config values to be overridden.
	RequestOption func(ro *requestOptions)

	requestOptions struct {
		header http.Header
		params url.Values
	}
)

// WithRequestHeader adds a header to the request.
func WithRequestHeader(k, v string) RequestOption {
	return func(ro *requestOptions) {
		ro.header.Add(k, v)
	}
}

// WithRequestParams adds a query parameter to the request.
func WithRequestParams(k, v string) RequestOption {
	return func(ro *requestOptions) {
		ro.params.Add(k, v)
	}
}

// mapRequestOptionsToDefaults applies the provided RequestOption to a default
// requestOptions struct.
func mapRequestOptionsToDefaults(opts []RequestOption) requestOptions {
	defaultOpts := requestOptions{}

	for _, opt := range opts {
		opt(&defaultOpts)
	}

	return defaultOpts
}

type (
	// ServerOption allows default server config values to be overridden.
	ServerOption func(so *serverOptions)

	serverOptions struct {
		address           string
		codec             ServerCodec
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

// WithServerCodec sets the ServerCodec that the Server will use by default when [NewHandler] is called.
func WithServerCodec(codec ServerCodec) ServerOption {
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
		codec:             NewJSONServerCodec(),
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
