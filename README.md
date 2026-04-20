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
- [Form Handlers](#form-handlers)
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
  - [JSON Handler with Request/Response](#json-handler-with-requestresponse)
  - [JSON Handler with Path Parameters](#json-handler-with-path-parameters)
  - [Basic net/http Handler](#basic-nethttp-handler)
  - [HTML Handler with Form Data](#html-handler-with-form-data)
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
- HTML template rendering with form data decoding for HTMX and traditional web apps
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
| ----------------------------- | ------- | ----------------------------------------------------- |
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

Parameters can be bound from different sources using the single `param` struct tag. This tag supports a comma-separated list of sources, allowing for sophisticated fallback strategies.

```go
type MyParams struct {
    // Simple binding: look for "id" in the path
    ID string `param:"path=id" validate:"required,uuid"`

    // Fallback strategy: try query param "filter", then header "X-Filter"
    Filter string `param:"query=filter,header=X-Filter"`

    // Default value: try header "X-API-Key", if missing use "default-key"
    APIKey string `param:"header=X-API-Key,default=default-key"`

    // Complex chain: Query -> Header -> Default
    Version int `param:"query=v,header=X-Version,default=1" validate:"min=1"`
}
```

**Supported Sources:**

- `path`: URL path parameters (e.g., `/users/{id}`).
- `query`: URL query string parameters (e.g., `?filter=active`).
- `header`: HTTP request headers (e.g., `X-API-Key: abc`).
- `default`: A static default value if no other sources match.

**Binding Strategy:**

1.  **First Match Wins:** The `param` tag is processed from left to right. The first source that provides a non-empty value is used.
    - _Example:_ `param:"query=id,header=X-ID,default=1"`
    - Check query parameter `id`. If present, use it.
    - If not, check header `X-ID`. If present, use it.
    - If neither is found, use the default value `1`.
2.  **Default is Terminal:** If a `default` source is reached, it is always used, and subsequent sources in the tag are ignored.
3.  **Validation on Defaults:** Parameters populated from a `default` source are **excluded from validation**. This allows developers to set safe defaults (e.g., `default=0` for an integer) without triggering validation errors (e.g., `min=1`) that would confusingly blame the client.
4.  **Error Reporting:** Validation errors will correctly reflect the **actual source** key used to populate the parameter, providing clear feedback to the client.
    - _Example:_ Given `param:"query=q,header=H"`. If the value is missing from query `q` but provided in header `H`, and that value fails validation, the error response will indicate that parameter `H` is invalid.

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

When creating handlers with `httputil.NewHandler()` or `httputil.NewFormHandler()`, you can customize their behavior using the following options:

| Option                | Default | Description                                                    |
| --------------------- | ------- | -------------------------------------------------------------- |
| `WithHandlerCodec`    | nil     | Sets the codec used for request/response serialization         |
| `WithHandlerGuard`    | nil     | Sets a guard for request interception                          |
| `WithHandlerLogger`   | nil     | Sets the logger used by the handler                            |
| `WithHandlerMessages` | nil     | Sets a custom `MessageFunc` for validation error messages (i18n) |

Example with custom handler options:

```go
handler := httputil.NewHandler(
    myHandlerFunc,
    httputil.WithHandlerCodec(htmlCodec),  // or httputil.NewJSONServerCodec()
    httputil.WithHandlerGuard(myAuthGuard),
    httputil.WithHandlerLogger(logger),
)
```

If handler options are not specified, the handler will inherit settings from the server when registered.

## Form Handlers

`NewFormHandler` is a variant of `NewHandler` designed for HTML form workflows. Instead of automatically writing an RFC 7807 error response when binding or validation fails, it passes the errors to your action via `Request.Errors`, allowing you to re-render the form with inline validation messages.

```go
type CreateUserRequest struct {
    Name  string `form:"name"  validate:"required"`
    Email string `form:"email" validate:"required,email"`
}

server.Register(httputil.Endpoint{
    Method: http.MethodPost,
    Path:   "/users",
    Handler: httputil.NewFormHandler(
        func(r httputil.RequestData[CreateUserRequest]) (*httputil.Response, error) {
            if r.Errors.HasAny() {
                // Re-render the form with errors and the submitted data.
                return httputil.OK(FormPage{Data: r.Data, Errors: r.Errors})
            }

            // Validation passed — process the form.
            return httputil.SeeOther("/users")
        },
    ),
})
```

### BindErrors API

`BindErrors` aggregates validation and binding errors from request processing. Field keys use dot-separated paths matching struct tag names (e.g. `address.city` for a nested `City` field).

| Method    | Description                                                                                         |
| --------- | --------------------------------------------------------------------------------------------------- |
| `HasAny()` | Returns `true` if any data or parameter binding error occurred                                     |
| `Get(field)` | Returns the translated error message for a field (checks data errors first, then parameter errors) |
| `All()`   | Returns a flat map of all field-to-message errors (data errors take precedence on key collision)     |

`HasAny()` reports whether any error occurred during binding, but `Get` and `All` only contain entries for error types that can be mapped to individual fields. If the error is not a recognised validation or decode error (e.g. a malformed JSON body), `HasAny()` will return `true` while `Get` and `All` remain empty — inspect `BindErrors.Data` or `BindErrors.Params` directly in that case.

### Custom Error Messages (i18n)

Use `WithHandlerMessages` to provide a custom `MessageFunc` that controls user-facing validation messages. This works with both `NewHandler` (customising RFC 7807 constraint violation details) and `NewFormHandler` (customising `BindErrors.Get` and `BindErrors.All`). It applies to request body (Data) validation only — parameter validation messages are generated by the parameter binding pipeline.

```go
messages := httputil.WithHandlerMessages(func(tag, param string) string {
    switch tag {
    case "required":
        return "ce champ est obligatoire"
    case "email":
        return "adresse e-mail invalide"
    default:
        return "valeur invalide"
    }
})

// Works with form handlers (errors passed to action via Request.Errors):
httputil.NewFormHandler(action, messages)

// Works with standard handlers (errors written as RFC 7807 responses):
httputil.NewHandler(action, messages)
```

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

// Or apply several middlewares in one call. They run in the order given:
// loggingMiddleware runs first, then authMiddleware, then the handler.
server.Register(endpoints.WithMiddleware(
    loggingMiddleware(logger),
    authMiddleware(authenticator),
)...)
```

Within a single `WithMiddleware` call, middlewares run in the order given (first arg runs first). Across chained
calls, the most recent call wraps the previous one, so it runs first — this lets you compose nested groups so an outer
group's middleware wraps everything from inner groups:

```go
admin := adminEndpoints.WithMiddleware(authMiddleware(authenticator)) // auth on admin only
all := append(httputil.EndpointGroup{}, admin...)
all = append(all, publicEndpoints...)

// loggingMiddleware wraps everything; authMiddleware still only runs on admin endpoints.
// On admin requests: log -> auth -> handler. On public requests: log -> handler.
server.Register(all.WithMiddleware(loggingMiddleware(logger))...)
```

Note that this LIFO across-call ordering is intentionally different from `WithClientInterceptor`, which uses FIFO across
calls because client interceptors form a flat chain rather than a nested composition.

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

    // curl -iS -X POST -H "Content-Type: application/json" -d '{"name": "Nick"}' localhost:8080/greetings
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
        Name string `param:"path=name" validate:"required"`
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

### HTML Handler with Form Data

Use `HTMLServerCodec` to build endpoints that accept form submissions and render HTML templates. This is ideal for HTMX-powered or traditional server-rendered web applications. The codec accepts any `TemplateExecutor` — both `*template.Template` and `*TemplateSet` implement this interface.

```go
package main

import (
    "context"
    "html/template"
    "net/http"

    "github.com/nickbryan/slogutil"

    "github.com/nickbryan/httputil"
)

func main() {
    logger := slogutil.NewJSONLogger()

    tmpl := template.Must(template.New("app").Parse(""))
    template.Must(tmpl.New("greeting").Parse(`<p>Hello, {{.Name}}!</p>`))
    template.Must(tmpl.New("error").Parse(
        `<div class="error"><h1>{{.Title}}</h1><p>{{.Detail}}</p></div>`,
    ))

    server := httputil.NewServer(
        logger,
        httputil.WithServerCodec(httputil.NewHTMLServerCodec(
            tmpl,
            httputil.WithHTMLErrorTemplate(tmpl.Lookup("error")),
        )),
    )

    server.Register(newGreetingFormEndpoint(tmpl))

    server.Serve(context.Background())

    // curl -X POST -d 'name=Nick' localhost:8080/greet
    // <p>Hello, Nick!</p>
}

func newGreetingFormEndpoint(tmpl *template.Template) httputil.Endpoint {
    type formData struct {
        Name string `form:"name" validate:"required"`
    }

    return httputil.Endpoint{
        Method: http.MethodPost,
        Path:   "/greet",
        Handler: httputil.NewHandler(
            func(r httputil.RequestData[formData]) (*httputil.Response, error) {
                return httputil.OK(httputil.Template{
                    Name: "greeting",
                    Data: r.Data,
                })
            },
            httputil.WithHandlerCodec(httputil.NewHTMLServerCodec(tmpl)),
        ),
    }
}
```

#### Using TemplateSet for Page Isolation

When multiple pages define the same block names (e.g. `{{ define "content" }}`), use `TemplateSet` to give each page its own isolated copy of the base templates. Each entry in the set is cloned from the shared base, so block definitions in one page do not conflict with another.

```go
base := template.Must(template.New("").Parse(""))
template.Must(base.New("layout").Parse(
    `<html><body>{{ block "content" . }}{{ end }}</body></html>`))
template.Must(base.New("error").Parse(
    `<html><body><h1>{{ .Title }}</h1><p>{{ .Detail }}</p></body></html>`))

ts, err := httputil.NewTemplateSet(base, map[string]string{
    "home":    `{{ template "layout" . }}{{ define "content" }}<h1>{{ .Title }}</h1>{{ end }}`,
    "about":   `{{ template "layout" . }}{{ define "content" }}<p>{{ .Body }}</p>{{ end }}`,
})
if err != nil {
    log.Fatal(err)
}

codec := httputil.NewHTMLServerCodec(ts,
    httputil.WithHTMLErrorTemplate(ts.Lookup("error")),
)
```

The `HTMLServerCodec` decodes `application/x-www-form-urlencoded` and `multipart/form-data` text fields using a `FormDecoder` interface (defaulting to [`go-playground/form`](https://github.com/go-playground/form)), and renders responses through Go's `html/template` package. File uploads are not handled by the codec; use `r.FormFile()` or `r.MultipartReader()` directly in your action handler. Pass an `httputil.Template{Name, Data}` as the response data to execute a specific named template from the template set.

**HTML Codec Options:**

| Option                       | Default                    | Description                                                                                   |
| ---------------------------- | -------------------------- | --------------------------------------------------------------------------------------------- |
| `WithHTMLErrorTemplate`      | Minimal default error page | Sets a `*template.Template` for error pages (receives `*problem.DetailedError` as its data)   |
| `WithHTMLFormDecoder`        | `go-playground/form`       | Sets a custom `FormDecoder` implementation for form data parsing                              |
| `WithHTMLMultipartMaxMemory` | 32 MB                      | Sets max memory used when parsing multipart/form-data forms                                   |

Error templates always receive a `*problem.DetailedError` as their data, providing access to `Title`, `Detail`, `Status`, `Type`, `Code`, `Instance`, and `ExtensionMembers`. When the original error is not a `DetailedError`, one is constructed from the HTTP status code.

#### Loading Templates from Disk

In a real application, templates typically live on disk (or in an embedded filesystem) rather than inline strings. The pattern is to parse shared layouts and partials into a base template, then read page sources into the map for `NewTemplateSet`. Error pages are just regular pages — they define their own blocks and use the shared layout like any other page.

```
templates/
  layouts/
    base.html        # shared layout with {{ block "content" . }}
  partials/
    nav.html         # reusable partial
  pages/
    home.html        # defines "content" block
    about.html       # defines "content" block (no conflict with home)
    error.html       # error page, also defines "content" block
```

```go
//go:embed templates
var templateFS embed.FS

func loadTemplates() (*httputil.TemplateSet, error) {
    fsys, _ := fs.Sub(templateFS, "templates")

    // Parse layouts and partials into the shared base. Use path-based names
    // (e.g. "layouts/base.html") so page templates can reference them.
    base := template.New("")

    fs.WalkDir(fsys, "layouts", func(path string, d fs.DirEntry, err error) error {
        if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
            return err
        }
        src, _ := fs.ReadFile(fsys, path)
        template.Must(base.New(path).Parse(string(src)))
        return nil
    })

    fs.WalkDir(fsys, "partials", func(path string, d fs.DirEntry, err error) error {
        if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
            return err
        }
        src, _ := fs.ReadFile(fsys, path)
        template.Must(base.New(path).Parse(string(src)))
        return nil
    })

    // Read page sources into a map. Each page gets its own clone of base.
    // Names use the full path relative to the filesystem root (e.g. "pages/home.html").
    pages := make(map[string]string)

    entries, _ := fs.ReadDir(fsys, "pages")
    for _, e := range entries {
        if e.IsDir() || filepath.Ext(e.Name()) != ".html" {
            continue
        }
        name := filepath.Join("pages", e.Name())
        src, _ := fs.ReadFile(fsys, name)
        pages[name] = string(src)
    }

    return httputil.NewTemplateSet(base, pages)
}
```

Wire the error page into the codec using `Lookup`:

```go
ts, err := loadTemplates()
if err != nil {
    log.Fatal(err)
}

codec := httputil.NewHTMLServerCodec(ts,
    httputil.WithHTMLErrorTemplate(ts.Lookup("pages/error.html")),
)
```

A runnable version of this pattern is available in the `ExampleNewTemplateSet_fromDisk` test.

### Advanced Examples

#### Combined Data and Parameters

```go
func userEndpoint() httputil.Endpoint {
    type (
        params struct {
            ID string `param:"path=id" validate:"required,uuid"`
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
standard `net/http.Client` and offers middleware via interceptors, base path management, request building, and 
request body encoding. Response handling uses standard `*http.Response`, making it fully compatible with
the `bodyclose` linter and idiomatic Go patterns.

### Creating a Client

You can create a new `Client` instance using `httputil.NewClient` and configure it with `ClientOption`s:

```go
client := httputil.NewClient(
    httputil.WithClientBasePath("https://api.example.com"),
    httputil.WithClientCookieJar(nil), // Or provide a custom http.CookieJar.
    httputil.WithClientInterceptor(NewLogInterceptor(logger)), // Add middleware.
    httputil.WithClientTimeout(10 * time.Second),
)
```

### Making Requests

The `Client` provides methods for common HTTP verbs. All methods return a `*http.Response` and an `error`.

```go
// GET request
resp, err := client.Get(
    context.Background(),
    "/users/123",
    httputil.WithRequestHeader("Authorization", "Bearer token"),
    httputil.WithRequestParam("version", "v1"),
)
if err != nil {
    return fmt.Errorf("making GET request: %w", err)
}
defer resp.Body.Close()

// POST request with a JSON body
type CreateUserRequest struct {
    Name string `json:"name"`
}

reqBody := CreateUserRequest{Name: "John Doe"}

resp, err = client.Post(context.Background(), "/users", reqBody)
if err != nil {
    return fmt.Errorf("making POST request: %w", err)
}
defer resp.Body.Close()

// PUT, PATCH, DELETE methods are similar
resp, err = client.Put(context.Background(), "/users/123", reqBody)
resp, err = client.Patch(context.Background(), "/users/123", reqBody)
resp, err = client.Delete(context.Background(), "/users/123")
```

The configured encoder (default JSON) is applied to request bodies only — it sets
`Content-Type` and encodes the body passed to `Post`/`Put`/`Patch`. Set `Accept`
yourself with `WithRequestHeader` when the server needs it. Response decoding is
left to the caller because the response often depends on the status code (e.g.
your type on 2xx, an RFC 7807 problem document on 4xx/5xx), which is knowledge a
generic codec cannot capture cleanly.

### Production Example

A complete example showing how to build a typed API client function with proper error handling and
RFC 7807 problem details. Use `problem.Response` to detect problem responses by content type rather
than enumerating status codes:

```go
package apiclient

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "log/slog"
    "net/http"

    "github.com/nickbryan/httputil"
    "github.com/nickbryan/httputil/problem"
)

// Problem type URIs defined by the API. These match the type field in the
// server's RFC 9457 problem responses and are used to identify specific errors.
const TypeUserNotFound = "https://api.example.com/problems/user-not-found"

// APIError represents an error response from the API containing RFC 7807 problem details to be used in the handler.
type APIError struct {
    Problem problem.DetailedError
}

func (e *APIError) Error() string {
    return "API error: " + e.Problem.Error()
}

// UnexpectedAPIResponseError is returned when the API responds with an unhandled status code.
type UnexpectedAPIResponseError struct {
    StatusCode int
}

func (e *UnexpectedAPIResponseError) Error() string {
    return fmt.Sprintf("unexpected API response status: %d", e.StatusCode)
}

type User struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

func GetUser(ctx context.Context, client *httputil.Client, id string) (user *User, err error) {
    resp, err := client.Get(ctx, "/users/"+id)
    if err != nil {
        return nil, fmt.Errorf("requesting user %s: %w", id, err)
    }
    defer func() {
        if e := resp.Body.Close(); e != nil {
            err = errors.Join(err, fmt.Errorf("closing response body: %w", e))
        }
    }()

    if problem.Response(resp) {
        var pd problem.DetailedError
        if err = json.NewDecoder(resp.Body).Decode(&pd); err != nil {
            return nil, fmt.Errorf("decoding problem response: %w", err)
        }

        // Match on the problem type URI to handle specific errors.
        if pd.Type == TypeUserNotFound {
            return nil, fmt.Errorf("user %s not found", id)
        }

        return nil, &APIError{Problem: pd}
    }

    if resp.StatusCode != http.StatusOK {
        return nil, &UnexpectedAPIResponseError{StatusCode: resp.StatusCode}
    }

    user = &User{}
    if err = json.NewDecoder(resp.Body).Decode(user); err != nil {
        return nil, fmt.Errorf("decoding user response: %w", err)
    }

    return user, nil
}

// Caller example:
func handleGetUser(ctx context.Context, client *httputil.Client, logger *slog.Logger) {
    user, err := GetUser(ctx, client, "123")
    if err != nil {
        if apiErr, ok := errors.AsType[*APIError](err); ok {
            logger.ErrorContext(ctx, "API error fetching user", slog.Any("problem", apiErr.Problem))
            return
        }

        logger.ErrorContext(ctx, "Unexpected error fetching user", slog.Any("error", err))
        return
    }

    logger.InfoContext(ctx, "fetched user", slog.String("name", user.Name))
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
provide becomes the outermost wrapper. `WithClientInterceptor` is variadic and may be called multiple times; in either
case the order is the same — earlier interceptors run first on each request, then later ones, then the underlying
transport.

This FIFO across-call ordering is intentionally different from `EndpointGroup.WithMiddleware`, which uses LIFO across
calls so that outer endpoint groups wrap inner ones. Client interceptors don't have a nested-group concept — they're a
single flat chain on one Client — so listing them in invocation order reads more naturally.

Basic rules and recommendations:

- Keep interceptors small and focused (single responsibility).
- Avoid modifying the incoming \*http.Request in place; use req = req.WithContext(...) or req.Clone(...) when changing it.
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
                "Client request started",
                slog.String("method", req.Method),
                slog.String("url", req.URL.String()),
            )

            resp, err := next.RoundTrip(req)

            attrs := []slog.Attr{
                slog.String("method", req.Method),
                slog.String("url", req.URL.String()),
                slog.Duration("duration", time.Since(start)),
            }
            if resp != nil {
                attrs = append(attrs, slog.Int("status", resp.StatusCode))
            }
            if err != nil {
                attrs = append(attrs, slog.Any("error", err))
            }
            logger.InfoContext(req.Context(), "Client request completed", attrs...)

            return resp, err
        })
    }
}
```

### Client Options

`httputil.NewClient` accepts `ClientOption`s to customize the underlying `http.Client`:

| Option                     | Default                 | Description                                                        |
|----------------------------|-------------------------|--------------------------------------------------------------------|
| `WithClientBasePath`       | `""`                    | Sets a base URL path for all requests                              |
| `WithClientEncoder`        | JSON                    | Sets the encoder for request body encoding and Content-Type        |
| `WithClientCookieJar`      | nil                     | Sets the `http.CookieJar` for the client                           |
| `WithClientTransport`      | `http.DefaultTransport` | Sets the base transport for the client                             |
| `WithClientInterceptor`    | none                    | Wraps the base transport to provide client middleware              |
| `WithClientTimeout`        | 60s                     | Sets the total timeout for requests                                |
| `WithClientRedirectPolicy` | nil                     | Sets the redirect policy for the client                            |

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
