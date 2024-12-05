package httputil

import (
	"time"
)

type (
	ServerOption func(so *serverOptions)

	serverOptions struct {
		idleTimeout       time.Duration
		readHeaderTimeout time.Duration
		readTimeout       time.Duration
		writeTimeout      time.Duration
		shutdownTimeout   time.Duration
	}
)

func WithIdleTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.idleTimeout = timeout
	}
}

func WithReadHeaderTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readHeaderTimeout = timeout
	}
}

func WithReadTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.readTimeout = timeout
	}
}

func WithShutdownTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.shutdownTimeout = timeout
	}
}

func WithWriteTimeout(timeout time.Duration) ServerOption {
	return func(so *serverOptions) {
		so.writeTimeout = timeout
	}
}

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
