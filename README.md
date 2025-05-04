# httputil
Package `httputil` provides utility helpers for working with `net/http`, adding sensible defaults, bootstrapping, 
and eliminating boilerplate code commonly required when building web services. This package aims to streamline the 
development of HTTP-based applications by offering a cohesive set of tools for HTTP server configuration, request 
handling, error management, and more.

<div align="center">

[![Test](https://github.com/nickbryan/httputil/actions/workflows/test.yml/badge.svg)](https://github.com/nickbryan/httputil/actions)
[![Coverage](https://raw.githubusercontent.com/nickbryan/httputil/badges/.badges/main/coverage.svg)](https://github.com/nickbryan/httputil/actions)
[![Go Report Card](https://goreportcard.com/badge/nickbryan/httputil)](https://goreportcard.com/report/nickbryan/httputil)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/nickbryan/httputil/blob/master/LICENSE)

</div>

- [Features](#features)
    - [HTTP Server with Sensible Defaults](#http-server-with-sensible-defaults)
    - [Handler Framework](#handler-framework)
    - [Error Handling](#error-handling)
    - [Request Parameter Processing](#request-parameter-processing)
    - [Testing Utilities](#testing-utilities)
- [Installation](#installation)
- [Server Options](#server-options)
- [Usage](#usage)
    - [Basic JSON Handler](#basic-json-handler)
    - [JSON Handler Request/Response](#json-handler-requestresponse)
    - [JSON Handler With Params](#json-handler-with-params)
    - [Basic `net/http` Handler](#basic-nethttp-handler)
- [Design Choices](#design-choices)

## Features

### HTTP Server with Sensible Defaults

- Configurable HTTP server with secure, production-ready defaults
- Graceful shutdown handling
- Customizable timeouts (idle, read, write, shutdown)
- Request body size limits to prevent abuse

### Handler Framework

- Easy conversion between different handler types
- Support for standard `http.Handler` interfaces
- JSON request/response handling with automatic marshaling/unmarshaling
- Request interception and middleware support

### Error Handling

- RFC 7807 compliant problem details for HTTP APIs
- Standardized error formatting
- Predefined error constructors for common HTTP status codes

### Request Parameter Processing

- Safe and convenient parameter extraction from different sources (URL, query, headers, body)
- Validation support

### Testing Utilities

- JSON comparison tools for testing HTTP responses
- Helper functions to reduce test boilerplate

## Installation

```bash
go get github.com/nickbryan/httputil
```

## Server Options
`httputil.NewServer` can be configured with the following options:

| Option                  | Default | Description                                           |
|-------------------------|---------|-------------------------------------------------------|
| `WithAddress`           | `:8080` | Sets the address the server will listen on            |
| `WithIdleTimeout`       | 30s     | Controls how long connections are kept open when idle |
| `WithMaxBodySize`       | 5MB     | Maximum allowed request body size                     |
| `WithReadHeaderTimeout` | 5s      | Maximum time to read request headers                  |
| `WithReadTimeout`       | 60s     | Maximum time to read the entire request               |
| `WithShutdownTimeout`   | 30s     | Time to wait for connections to close during shutdown |
| `WithWriteTimeout`      | 30s     | Maximum time to write a response                      |

## Usage

### Basic JSON Handler

```go
package main

import (
    "context"
    "net/http"

    "github.com/nickbryan/slogutil"

    "github.com/nickbryan/httputil"
)

func main() {
    logger := slogutil.NewJSONLogger()  
    server := httputil.NewServer(logger)
	
    server.Register(
        httputil.Endpoint{
            Method: http.MethodGet, 
            Path:   "/greetings", 
            Handler: httputil.NewJSONHandler(
                func(_ httputil.RequestEmpty) (*httputil.Response, error) {
                    return httputil.OK([]string{"Hello, World!", "Hola Mundo!"})
                }, 
            ),
        }, 
    )

    server.Serve(context.Background())

    // curl localhost:8080/greetings
    // ["Hello, World!","Hola Mundo!"]
}
```

### JSON Handler Request/Response

```go
package main

import (
    "context"
    "net/http"

    "github.com/nickbryan/slogutil"

    "github.com/nickbryan/httputil"
)

func main() {
    logger := slogutil.NewJSONLogger()
    server := httputil.NewServer(logger)

    server.Register(newGreetingsEndpoint())

    server.Serve(context.Background())

    // curl -iS -X POST -H "Content-Type: application/json" -d '{"name": "Nick"}' localhost:8080/greetings                                                                               7 ↵
    // HTTP/1.1 201 Created
    // Content-Type: application/json
    // Date: Sat, 29 Mar 2025 17:12:40 GMT
    // Content-Length: 26
    //
    // {"message":"Hello Nick!"}
}

func newGreetingsEndpoint() httputil.Endpoint {
    type (
        request struct {
            Name string `json:"name" validate:"required"`
        }
        response struct {
            Message string `json:"message"`
        }
    )

    return httputil.Endpoint{
        Method: http.MethodPost,
        Path:   "/greetings",
        Handler: httputil.NewJSONHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
            return httputil.Created(response{Message: "Hello " + r.Data.Name + "!"})
        }),
    }
}
```

### JSON Handler With Params

```go
package main

import (
    "context"
    "net/http"

    "github.com/nickbryan/slogutil"

    "github.com/nickbryan/httputil"
)

func main() {
    logger := slogutil.NewJSONLogger()
    server := httputil.NewServer(logger)

    server.Register(newGreetingsEndpoint())

    server.Serve(context.Background())

    // curl localhost:8080/greetings/Nick
    // ["Hello, Nick!","Hola Nick!"]
}

func newGreetingsEndpoint() httputil.Endpoint {
    type params struct {
        Name string `path:"name" validate:"required"`
    }

    return httputil.Endpoint{
        Method: http.MethodGet,
        Path:   "/greetings/{name}",
        Handler: httputil.NewJSONHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
            return httputil.OK([]string{"Hello, " + r.Params.Name + "!", "Hola " + r.Params.Name + "!"})
        }),
    }
}
```

### Basic `net/http` Handler

```go
package main

import (
    "context"
    "net/http"

    "github.com/nickbryan/slogutil"

    "github.com/nickbryan/httputil"
)

func main() {
    logger := slogutil.NewJSONLogger()
    server := httputil.NewServer(logger)

    server.Register(
        httputil.Endpoint{
            Method: http.MethodGet,
            Path:   "/greetings",
            Handler: httputil.NewNetHTTPHandlerFunc(
                func(w http.ResponseWriter, _ *http.Request) {
                    _, _ = w.Write([]byte(`["Hello, World!","Hola Mundo!"]`))
                },
            ),
        },
    )

    server.Serve(context.Background())
	
    // curl localhost:8080/greetings
    // ["Hello, World!","Hola Mundo!"]
}
```

## Design Choices

### RFC 7807 Problem Details
Error responses follow the RFC 7807 standard for Problem Details for HTTP APIs, providing consistent, readable 
error information.

### Middleware Architecture
Middleware can be applied at both the server and endpoint level, providing a flexible way to implement cross-cutting 
concerns like logging, authentication, and metrics.

### Handler Interfaces
The package provides a consistent interface for handlers while supporting multiple styles (standard http.Handler, 
functional handlers, and JSON-specific handlers).

## TODO
- migrate run and satisfy linter.
- mock the encoder for these tests then test the encoder	directly.
- test the default encoder with the server options.
- address endpoints API.
- address the net/http handler API.
- test view.
- tidy up the handler internals.
- address the RequestX API.
- revisit documentation.
