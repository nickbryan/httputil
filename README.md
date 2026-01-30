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

## Table of Contents
- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Server Configuration](#server-configuration)
- [Request Handling](#request-handling)
  - [Basic Handlers](#basic-handlers)
  - [Request Types](#request-types)
  - [Parameter Binding](#parameter-binding)
  - [Validation](#validation)
- [Handler Options](#handler-options)
- [Response Helpers](#response-helpers)
- [Error Handling](#error-handling)
  - [RFC 7807 Problem Details](#rfc-7807-problem-details)
  - [Predefined Error Types](#predefined-error-types)
- [Middleware](#middleware)
  - [Built-in Middleware](#built-in-middleware)
  - [Custom Middleware](#custom-middleware)
- [Guards](#guards)
  - [Request Interception](#request-interception)
  - [Guard Stacks](#guard-stacks)
- [Endpoint Groups](#endpoint-groups)
- [Testing](#testing)
- [Examples](#examples)
  - [Basic JSON Handler](#basic-json-handler)
  - [JSON Handler with Request/Response](#json-handler-requestresponse)
  - [JSON Handler with Path Parameters](#json-handler-with-path-parameters)
  - [Basic net/http Handler](#basic-nethttp-handler)
  - [Advanced Examples](#advanced-examples)
- [Client Usage](#client-usage)
- [Design Choices](#design-choices)
- [Contributing](#contributing)
- [License](#license)

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
- Validation support using the `validator` package

### Testing Utilities

- JSON comparison tools for testing HTTP responses
- Helper functions to reduce test boilerplate

## Installation

```bash
go get github.com/nickbryan/httputil
```

## Quick Start

Here's a minimal example to get you started:

```go
package main

import (
    "context"
    "net/http"
    "log/slog"
    "os"

    "github.com/nickbryan/httputil"
)

func main() {
    // Create a logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    
    // Create a server with default options
    server := httputil.NewServer(logger)
    
    // Register an endpoint
    server.Register(
        httputil.Endpoint{
            Method: http.MethodGet,
            Path:   "/hello",
            Handler: httputil.NewHandler(
                func(_ httputil.RequestEmpty) (*httputil.Response, error) {
                    return httputil.OK(map[string]string{"message": "Hello, World!"})
                },
            ),
        },
    )
    
    // Start the server
    server.Serve(context.Background())
}
```

## Server Configuration

`httputil.NewServer` can be configured with the following options:

| Option                        | Default | Description                                           |
|-------------------------------|---------|-------------------------------------------------------|
| `WithServerAddress`           | `:8080` | Sets the address the server will listen on            |
| `WithServerCodec`             | JSON    | Sets the default codec for request/response encoding  |
| `WithServerIdleTimeout`       | 30s     | Controls how long connections are kept open when idle |
| `WithServerMaxBodySize`       | 5MB     | Maximum allowed request body size                     |
| `WithServerReadHeaderTimeout` | 5s      | Maximum time to read request headers                  |
| `WithServerReadTimeout`       | 60s     | Maximum time to read the entire request               |
| `WithServerShutdownTimeout`   | 30s     | Time to wait for connections to close during shutdown |
| `WithServerWriteTimeout`      | 30s     | Maximum time to write a response                      |

Example with custom configuration:

```go
server := httputil.NewServer(
    logger,
    httputil.WithServerAddress(":3000"),
    httputil.WithServerMaxBodySize(10 * 1024 * 1024), // 10MB
    httputil.WithServerReadTimeout(30 * time.Second),
)
```

## Request Handling

### Basic Handlers

The package provides a flexible handler system that supports different request types:

```go
// Empty request (no body or parameters)
httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
    return httputil.OK(map[string]string{"message": "Hello, World!"})
})

// Request with JSON body
httputil.NewHandler(func(r httputil.RequestData[MyRequestType]) (*httputil.Response, error) {
    // Access request data with r.Data
    return httputil.OK(map[string]string{"message": "Hello, " + r.Data.Name})
})

// Request with path/query parameters
httputil.NewHandler(func(r httputil.RequestParams[MyParamsType]) (*httputil.Response, error) {
    // Access parameters with r.Params
    return httputil.OK(map[string]string{"message": "Hello, " + r.Params.Name})
})
```

### Request Types

The package supports three main request types:

1. `RequestEmpty` - For requests without body or parameters
2. `RequestData<T>` - For requests with a JSON body of type T
3. `RequestParams<P>` - For requests with path/query parameters of type P

You can also combine both data and parameters:

```go
httputil.NewHandler(func(r httputil.Request[MyRequestType, MyParamsType]) (*httputil.Response, error) {
    // Access both r.Data and r.Params
    return httputil.OK(map[string]string{
        "message": "Hello, " + r.Params.Name,
        "details": r.Data.Details,
    })
})
```

### Parameter Binding

Parameters can be bound from different sources using struct tags:

```go
type MyParams struct {
    ID      string `path:"id" validate:"required,uuid"`
    Filter  string `query:"filter"`
    APIKey  string `header:"X-API-Key" validate:"required"`
    Version int    `query:"version" validate:"required,min=1"`
}
```

Supported parameter sources:
- `path` - URL path parameters
- `query` - Query string parameters
- `header` - HTTP headers

### Validation

The package uses [go-playground/validator](https://github.com/go-playground/validator) for request validation:

```go
type CreateUserRequest struct {
    Name     string `json:"name" validate:"required,min=2,max=100"`
    Email    string `json:"email" validate:"required,email"`
    Age      int    `json:"age" validate:"required,min=18"`
    Password string `json:"password" validate:"required,min=8"`
}
```

Validation errors are automatically converted to RFC 7807 problem details responses.

## Handler Options

When creating handlers with `httputil.NewHandler()`, you can customize their behavior using the following options:

| Option                | Default | Description                                           |
|-----------------------|---------|-------------------------------------------------------|
| `WithHandlerCodec`    | nil     | Sets the codec used for request/response serialization |
| `WithHandlerGuard`    | nil     | Sets a guard for request interception                  |
| `WithHandlerLogger`   | nil     | Sets the logger used by the handler                    |

Example with custom handler options:

```go
handler := httputil.NewHandler(
    myHandlerFunc,
    httputil.WithHandlerCodec(httputil.NewJSONCodec()),
    httputil.WithHandlerGuard(myAuthGuard),
    httputil.WithHandlerLogger(logger),
)
```

If handler options are not specified, the handler will inherit settings from the server when registered.

## Response Helpers

The package provides helper functions for creating common HTTP responses:

```go
// 200 OK
httputil.OK(data)

// 201 Created
httputil.Created(data)

// 202 Accepted
httputil.Accepted(data)

// 204 No Content
httputil.NoContent()

// 301/302/307/308 Redirects
httputil.Redirect(http.StatusTemporaryRedirect, "/new-location")
```

For custom status codes, use `NewResponse`:

```go
httputil.NewResponse(http.StatusPartialContent, data)
```

## Error Handling

### RFC 7807 Problem Details

Error responses follow the [RFC 7807](https://tools.ietf.org/html/rfc7807) standard for Problem Details for HTTP APIs:

```json
{
  "type": "https://example.com/problems/constraint-violation",
  "title": "Constraint Violation",
  "status": 400,
  "detail": "The request body contains invalid fields",
  "code": "INVALID_REQUEST_BODY",
  "instance": "/users",
  "invalid_params": [
    {
      "name": "email",
      "reason": "must be a valid email address"
    }
  ]
}
```

### Predefined Error Types

The package provides predefined error constructors for common HTTP status codes:

```go
// 400 Bad Request
problem.BadRequest("Invalid request format")

// 401 Unauthorized
problem.Unauthorized("Authentication required")

// 403 Forbidden
problem.Forbidden("Insufficient permissions")

// 404 Not Found
problem.NotFound("User not found")

// 409 Conflict
problem.ResourceExists("User already exists")

// 422 Unprocessable Entity
problem.ConstraintViolation("Invalid input", []problem.Parameter{
    {Name: "email", Reason: "must be a valid email address"},
})

// 500 Internal Server Error
problem.ServerError("An unexpected error occurred")
```

## Middleware

### Built-in Middleware

The package includes built-in middleware for common tasks:

1. **Panic Recovery** - Automatically recovers from panics in handlers
2. **Max Body Size** - Limits request body size to prevent abuse

These are applied automatically by the server.

### Custom Middleware

You can create custom middleware using the `MiddlewareFunc` type:

```go
func loggingMiddleware(logger *slog.Logger) httputil.MiddlewareFunc {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            // Call the next handler
            next.ServeHTTP(w, r)
            
            // Log after the request is processed
            logger.InfoContext(r.Context(), "Request processed",
                slog.String("method", r.Method),
                slog.String("path", r.URL.Path),
                slog.Duration("duration", time.Since(start)),
            )
        })
    }
}
```

Apply middleware to endpoints:

```go
endpoints := httputil.EndpointGroup{
    httputil.Endpoint{
        Method: http.MethodGet,
        Path:   "/users",
        Handler: httputil.NewHandler(listUsers),
    },
    httputil.Endpoint{
        Method: http.MethodPost,
        Path:   "/users",
        Handler: httputil.NewHandler(createUser),
    },
}

// Apply middleware to all endpoints in the group
server.Register(endpoints.WithMiddleware(loggingMiddleware(logger))...)
```

## Guards

Guards provide a way to intercept and potentially modify requests before they reach handlers.

### Request Interception

Implement the `Guard` interface:

```go
type AuthGuard struct {
    secretKey string
}

func (g *AuthGuard) Guard(r *http.Request) (*http.Request, error) {
    token := r.Header.Get("Authorization")
    if token == "" {
        return nil, problem.Unauthorized("Missing authorization token")
    }
    
    // Validate token...
    
    // Add user info to context
    ctx := context.WithValue(r.Context(), "user", userInfo)
    return r.WithContext(ctx), nil
}
```

Apply the guard to an endpoint:

```go
endpoint := httputil.NewEndpointWithGuard(
    httputil.Endpoint{
        Method: http.MethodGet,
        Path:   "/protected",
        Handler: httputil.NewHandler(protectedHandler),
    },
    &AuthGuard{secretKey: "your-secret-key"},
)
```

### Guard Stacks

Combine multiple guards using `GuardStack`:

```go
guards := httputil.GuardStack{
    &RateLimitGuard{},
    &AuthGuard{secretKey: "your-secret-key"},
    &LoggingGuard{logger: logger},
}

endpoint := httputil.NewEndpointWithGuard(
    httputil.Endpoint{
        Method: http.MethodGet,
        Path:   "/protected",
        Handler: httputil.NewHandler(protectedHandler),
    },
    guards,
)
```

## Endpoint Groups

`EndpointGroup` allows you to manage multiple endpoints together:

```go
userEndpoints := httputil.EndpointGroup{
    httputil.Endpoint{
        Method: http.MethodGet,
        Path:   "/users",
        Handler: httputil.NewHandler(listUsers),
    },
    httputil.Endpoint{
        Method: http.MethodPost,
        Path:   "/users",
        Handler: httputil.NewHandler(createUser),
    },
}

// Add a path prefix to all endpoints
prefixedEndpoints := userEndpoints.WithPrefix("/api/v1")

// Apply middleware to all endpoints
secureEndpoints := prefixedEndpoints.WithMiddleware(authMiddleware)

// Apply a guard to all endpoints
guardedEndpoints := secureEndpoints.WithGuard(&RateLimitGuard{})

// Register all endpoints
server.Register(guardedEndpoints...)
```

## Testing

The package provides utilities for testing HTTP handlers:

```go
func TestUserHandler(t *testing.T) {
    handler := httputil.NewHandler(func(r httputil.RequestEmpty) (*httputil.Response, error) {
        return httputil.OK(map[string]string{"message": "Hello, World!"})
    })
    
    req := httptest.NewRequest(http.MethodGet, "/users", nil)
    w := httptest.NewRecorder()
    
    handler.ServeHTTP(w, req)
    
    if w.Code != http.StatusOK {
        t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
    }
    
    // Use the testutil package for JSON comparison
    expected := `{"message":"Hello, World!"}`
    if err := testutil.JSONEquals(expected, w.Body.String()); err != nil {
        t.Error(err)
    }
}
```

## Examples

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
            Handler: httputil.NewHandler(
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

### JSON Handler with Request/Response

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
        Handler: httputil.NewHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
            return httputil.Created(response{Message: "Hello " + r.Data.Name + "!"})
        }),
    }
}
```

### JSON Handler with Path Parameters

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
        Handler: httputil.NewHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
            return httputil.OK([]string{"Hello, " + r.Params.Name + "!", "Hola " + r.Params.Name + "!"})
        }),
    }
}
```

### Basic net/http Handler

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
            Handler: httputil.WrapNetHTTPHandlerFunc(
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

### Advanced Examples

#### Combined Data and Parameters

```go
func userEndpoint() httputil.Endpoint {
    type (
        params struct {
            ID string `path:"id" validate:"required,uuid"`
        }
        request struct {
            Name  string `json:"name" validate:"required"`
            Email string `json:"email" validate:"required,email"`
        }
    )

    return httputil.Endpoint{
        Method: http.MethodPut,
        Path:   "/users/{id}",
        Handler: httputil.NewHandler(func(r httputil.Request[request, params]) (*httputil.Response, error) {
            // Access both r.Data and r.Params
            return httputil.OK(map[string]string{
                "id": r.Params.ID,
                "name": r.Data.Name,
                "email": r.Data.Email,
            })
        }),
    }
}
```

#### Custom Middleware and Guards

```go
func setupServer() *httputil.Server {
    logger := slogutil.NewJSONLogger()
    server := httputil.NewServer(logger)

    // Create middleware
    loggingMiddleware := func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            logger.InfoContext(r.Context(), "Request started", 
                slog.String("method", r.Method), 
                slog.String("path", r.URL.Path))
            next.ServeHTTP(w, r)
        })
    }

    // Create guard
    authGuard := httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
        token := r.Header.Get("Authorization")
        if token == "" {
            return nil, problem.Unauthorized("Missing authorization token")
        }
        return r, nil
    })

    // Create endpoints
    endpoints := httputil.EndpointGroup{
        httputil.Endpoint{
            Method: http.MethodGet,
            Path:   "/users",
            Handler: httputil.NewHandler(listUsers),
        },
        httputil.Endpoint{
            Method: http.MethodPost,
            Path:   "/users",
            Handler: httputil.NewHandler(createUser),
        },
    }

    // Apply middleware and guard
    secureEndpoints := endpoints.
        WithMiddleware(loggingMiddleware).
        WithGuard(authGuard).
        WithPrefix("/api/v1")

    server.Register(secureEndpoints...)
    
    return server
}
```

## Client Usage

`httputil.Client` provides a convenient and idiomatic way to make HTTP requests to external services. It wraps the 
standard `net/http.Client` and offers simplified methods for common HTTP operations, along with robust response handling.

### Creating a Client

You can create a new `Client` instance using `httputil.NewClient` and configure it with `ClientOption`s:

```go
client := httputil.NewClient(
    httputil.WithClientBasePath("https://api.example.com"),
    httputil.WithClientCookieJar(nil), // Or provide a custom http.CookieJar.
    httputil.WithClientInterceptor(NewLogInterceptor(logger)), // Add middleware.
    httputil.WithClientTimeout(10 * time.Second),
)
defer client.Close()
```

### Making Requests

The `Client` provides methods for common HTTP verbs. All methods return a `*httputil.Result` and an `error`.

```go
// GET request
resp, err := client.Get(
	context.Background(), 
	"/users/123",
    httputil.WithRequestHeader("Authorization", "Bearer token"),
    httputil.WithRequestParam("version", "v1"),
)
if err != nil {
    fmt.Printf("Error making GET request: %v\n", err)
}

// POST request with a JSON body
type MyRequest struct {
    Name string `json:"name"`
}
reqBody := MyRequest{Name: "John Doe"}

resp, err = client.Post(context.Background(), "/users", reqBody)
if err != nil {
    fmt.Printf("Error making POST request: %v\n", err)
}

// PUT, PATCH, DELETE methods are similar
resp, err = client.Put(context.Background(), "/users/123", reqBody)
resp, err = client.Patch(context.Background(), "/users/123", reqBody)
resp, err = client.Delete(context.Background(), "/users/123")
```

### Handling Responses

The `*httputil.Result` type wraps the `*http.Response` and provides convenient methods for checking status codes and decoding the response body.

```go
type MyResponse struct {
    Message string `json:"message"`
}

// Check for success or error
if resp.IsSuccess() {
    var data MyResponse
    if err := resp.Decode(&data); err != nil {
        fmt.Printf("Error decoding success response: %v\n", err)
    }
	
    fmt.Printf("Success: %s\n", data.Message)
} else if resp.IsError() {
    // Decodes as RFC 7807 Problem Details
    problemDetails, err := resp.AsProblemDetails()
    if err != nil {
        fmt.Printf("Error decoding problem details: %v\n", err)
    }

    fmt.Printf("Error: %s - %s\n", problemDetails.Title, problemDetails.Detail)
} else {
    fmt.Printf("Unhandled status code: %d\n", resp.StatusCode)
}
```

### Client Middleware with Interceptors
The client uses an interceptor model that wraps the underlying http.RoundTripper. Interceptors let you run logic before 
and after an HTTP request is sent (logging, retries, tracing, auth headers, metrics, etc.) without changing call sites. 
An interceptor has the shape:

```go
type InterceptorFunc func(next http.RoundTripper) http.RoundTripper
```

Each interceptor receives the "next" RoundTripper and returns a new RoundTripper that calls next.RoundTrip(req) when 
appropriate. Interceptors are applied by wrapping the base transport so they form a chain: the first interceptor you 
provide becomes the outermost wrapper.

Basic rules and recommendations:
- Keep interceptors small and focused (single responsibility).
- Avoid modifying the incoming *http.Request in place; use req = req.WithContext(...) or req.Clone(...) when changing it.
- Ensure you always call next.RoundTrip unless you intentionally short-circuit (for example, returning a cached response or an error).
- Be mindful of retry/interceptor interactions (idempotency, body re-reads). If you need to retry requests with bodies, buffer them or use a replayable body.

**Example: simple logging interceptor**
```go
func NewLogInterceptor(logger *slog.Logger) httputil.InterceptorFunc {
    return func(next http.RoundTripper) http.RoundTripper {
        return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
            start := time.Now()
            logger.DebugContext(
				req.Context(), 
				s"Client request started",
                slog.String("method", req.Method),
                slog.String("url", req.URL.String()),
            )

            resp, err := next.RoundTrip(req)

            logger.InfoContext(
				req.Context(), 
				"Client request completed",
                slog.String("method", req.Method),
                slog.String("url", req.URL.String()),
                slog.Int("status", resp.StatusCode),
                slog.Duration("duration", time.Since(start)),
                slog.Any("error", err),
            )
			
            return resp, err
        })
    }
}
```

### Client Options

`httputil.NewClient` accepts `ClientOption`s to customize the underlying `http.Client`:

| Option                     | Default                 | Description                                                     |
|----------------------------|-------------------------|-----------------------------------------------------------------|
| `WithClientBasePath`       | `""`                    | Sets a base URL path for all requests                           |
| `WithClientCodec`          | JSON                    | Sets the codec for request/response serialization               |
| `WithClientCookieJar`      | nil                     | Sets the `http.CookieJar` for the client                        |
| `WithClientInterceptor`    | `http.DefaultTransport` | Wraps the `http.DefaultTransport` to provide client middleware. |
| `WithClientTimeout`        | 60s                     | Sets the total timeout for requests                             |
| `WithClientRedirectPolicy` | nil                     | Sets the redirect policy for the client                         |

### Request Options

Request-specific options can be passed to individual HTTP method calls:

| Option               | Description                                  |
|----------------------|----------------------------------------------|
| `WithRequestHeader`  | Adds a single HTTP header to the request     |
| `WithRequestHeaders` | Adds multiple HTTP headers from a map        |
| `WithRequestParam`   | Adds a single query parameter to the request |
| `WithRequestParams`  | Adds multiple query parameters from a map    |

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

### Type Safety with Generics
The package uses Go generics to provide type-safe request handling, ensuring that request data and parameters are properly typed.

### Graceful Shutdown
The server implementation includes graceful shutdown handling, ensuring that in-flight requests are completed before the server stops.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
