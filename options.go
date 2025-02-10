package httputil

import (
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

type (
	// ServerOption allows default config values to be overridden.
	ServerOption func(so *serverOptions)

	serverOptions struct {
		address           string
		idleTimeout       time.Duration
		readHeaderTimeout time.Duration
		readTimeout       time.Duration
		validator         *validator.Validate
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

// WithValidator sets the timeout for writing the response.
func WithValidator(validator *validator.Validate) ServerOption {
	return func(so *serverOptions) {
		so.validator = validator
	}
}

// WithWriteTimeout sets the timeout for writing the response.
func WithWriteTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.writeTimeout = timeout
	}
}

// mapServerOptionsToDefaults applies the provided ServerOptions to a default
// serverOptions struct.
func mapServerOptionsToDefaults(opts []ServerOption) serverOptions {
	const (
		defaultShutdownTimeout = 5 * time.Second
		defaultTimeout         = time.Minute
	)

	defaultOpts := serverOptions{
		address:           ":8080",
		idleTimeout:       defaultTimeout,
		readHeaderTimeout: defaultTimeout,
		readTimeout:       defaultTimeout,
		shutdownTimeout:   defaultShutdownTimeout,
		validator:         NewValidator(),
		writeTimeout:      defaultTimeout,
	}

	for _, opt := range opts {
		opt(&defaultOpts)
	}

	return defaultOpts
}
