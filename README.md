# httputil

Package `httputil` provides utility helpers for working with `net/http`, adding sensible defaults, bootstrapping,
and eliminating boilerplate code commonly required when building web services. This package aims to streamline the
development of HTTP-based applications by offering a cohesive set of tools for HTTP server configuration, request
handling, error management, and HTTP client interactions.

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
  - [Transformers](#transformers)
- [Handler Options](#handler-options)
- [Form Handlers](#form-handlers)
  - [BindErrors API](#binderrors-api)
  - [Custom Error Messages (i18n)](#custom-error-messages-i18n)
- [Response Helpers](#response-helpers)
- [HTML Templates](#html-templates)
  - [HTMLServerCodec](#htmlservercodec)
  - [TemplateSet for Page Isolation](#templateset-for-page-isolation)
  - [Loading Templates from Disk](#loading-templates-from-disk)
- [Error Handling](#error-handling)
  - [RFC 9457 Problem Details](#rfc-9457-problem-details)
  - [Predefined Error Constructors](#predefined-error-constructors)
- [Middleware](#middleware)
  - [Built-in Middleware](#built-in-middleware)
  - [Custom Middleware](#custom-middleware)
- [Guards](#guards)
  - [Request Interception](#request-interception)
  - [Guard Stacks](#guard-stacks)
- [Endpoint Groups](#endpoint-groups)
- [Wrapping Standard `http.Handler`](#wrapping-standard-httphandler)
- [Examples](#examples)
  - [Basic JSON Handler](#basic-json-handler)
  - [JSON Handler with Request/Response](#json-handler-with-requestresponse)
  - [JSON Handler with Path Parameters](#json-handler-with-path-parameters)
  - [Combined Data and Parameters](#combined-data-and-parameters)
  - [Basic net/http Handler](#basic-nethttp-handler)
  - [HTML Handler with Form Data](#html-handler-with-form-data)
  - [Custom Middleware and Guards](#custom-middleware-and-guards)
- [Client](#client)
  - [Creating a Client](#creating-a-client)
  - [Making Requests](#making-requests)
  - [The `Result[T]` Return Value](#the-resultt-return-value)
  - [Skipping Response Decoding](#skipping-response-decoding)
  - [Sending Raw Bodies](#sending-raw-bodies)
  - [Headers and Query Parameters](#headers-and-query-parameters)
  - [Error Handling](#client-error-handling)
  - [Custom Error Decoders](#custom-error-decoders)
  - [Interceptors (Client Middleware)](#interceptors-client-middleware)
  - [Escape Hatch: `Client.Do`](#escape-hatch-clientdo)
  - [Client Options](#client-options)
  - [Request Options](#request-options)
- [Design Choices](#design-choices)
- [Contributing](#contributing)
- [License](#license)

## Features

### HTTP Server with Sensible Defaults

- Configurable HTTP server with secure, production-ready defaults.
- Graceful shutdown handling on `SIGINT`, `SIGTERM`, and `SIGQUIT`.
- Customisable timeouts (idle, read, header read, write, shutdown).
- Built-in panic recovery and request body size limits.

### Handler Framework

- Type-safe request/response handling using Go generics.
- Built-in JSON request/response encoding via a pluggable codec.
- HTML codec with `html/template` rendering and form decoding for HTMX or
  traditional server-rendered apps.
- Form-friendly variant (`NewFormHandler`) that surfaces validation errors to
  the action so forms can re-render with inline messages.
- Standard `net/http.Handler` wrapper (`WrapNetHTTPHandler`) for incremental
  adoption.

### Structured Error Responses

- RFC 9457 compliant problem details.
- Predefined error constructors for common HTTP status codes.
- Codec-aware error encoding (JSON, HTML, etc.).

### Request Parameter Binding

- Single `param` struct tag with multi-source fallback (path → query → header
  → default).
- Validation via [go-playground/validator](https://github.com/go-playground/validator)
  with errors mapped back to the original request source.

### HTTP Client

- Type-safe generic helpers (`Get[T]`, `Post[T]`, etc.) with automatic
  body draining and closing.
- Automatic decoding of RFC 9457 problem responses into typed errors.
- Pluggable error decoders for non-RFC-9457 APIs.
- Interceptor (round-tripper middleware) chain for cross-cutting concerns
  such as logging, retries, tracing, and auth.

## Installation

```bash
go get github.com/nickbryan/httputil
```

This package targets Go 1.26+ (it uses `errors.AsType` and `wg.Go`).

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"

    "github.com/nickbryan/httputil"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    server := httputil.NewServer(logger)

    server.Register(httputil.Endpoint{
        Method: http.MethodGet,
        Path:   "/hello",
        Handler: httputil.NewHandler(
            func(_ httputil.RequestEmpty) (*httputil.Response, error) {
                return httputil.OK(map[string]string{"message": "Hello, World!"})
            },
        ),
    })

    server.Serve(context.Background())
}
```

## Server Configuration

`httputil.NewServer` takes a `*slog.Logger` and zero or more `ServerOption`s:

| Option                        | Default | Description                                           |
| ----------------------------- | ------- | ----------------------------------------------------- |
| `WithServerAddress`           | `:8080` | Sets the address the server will listen on            |
| `WithServerCodec`             | JSON    | Default codec for request decoding and response encoding |
| `WithServerIdleTimeout`       | 30s     | How long connections are kept open when idle          |
| `WithServerMaxBodySize`       | 5 MB    | Maximum allowed request body size                     |
| `WithServerReadHeaderTimeout` | 5s      | Maximum time to read request headers                  |
| `WithServerReadTimeout`       | 60s     | Maximum time to read the entire request               |
| `WithServerShutdownTimeout`   | 30s     | Time to wait for connections to close during shutdown |
| `WithServerWriteTimeout`      | 30s     | Maximum time to write a response                      |

Example with custom configuration:

```go
server := httputil.NewServer(
    logger,
    httputil.WithServerAddress(":3000"),
    httputil.WithServerMaxBodySize(10 * 1024 * 1024), // 10 MB
    httputil.WithServerReadTimeout(30 * time.Second),
)
```

`Server.Serve(ctx)` blocks until a termination signal is received (or `ctx`
is cancelled), then performs a graceful shutdown bounded by
`WithServerShutdownTimeout`. The server also implements `http.Handler` via
`ServeHTTP`, allowing endpoints to be exercised in tests with `httptest`
without binding to a port.

## Request Handling

### Basic Handlers

`httputil.NewHandler` builds a typed handler from an action that takes a
`Request[D, P]` and returns a `*Response`:

```go
// Empty request (no body, no parameters).
httputil.NewHandler(func(_ httputil.RequestEmpty) (*httputil.Response, error) {
    return httputil.OK(map[string]string{"message": "Hello, World!"})
})

// Request with a decoded body.
httputil.NewHandler(func(r httputil.RequestData[MyRequestType]) (*httputil.Response, error) {
    return httputil.OK(map[string]string{"message": "Hello, " + r.Data.Name})
})

// Request with bound path/query/header parameters.
httputil.NewHandler(func(r httputil.RequestParams[MyParamsType]) (*httputil.Response, error) {
    return httputil.OK(map[string]string{"message": "Hello, " + r.Params.Name})
})
```

### Request Types

There are three convenience aliases plus the underlying generic type:

| Type                  | Purpose                                |
| --------------------- | -------------------------------------- |
| `RequestEmpty`        | No body and no parameters              |
| `RequestData[D]`      | A decoded body of type `D`             |
| `RequestParams[P]`    | Bound parameters of type `P`           |
| `Request[D, P]`       | Both a body and parameters             |

```go
httputil.NewHandler(func(r httputil.Request[MyRequestType, MyParamsType]) (*httputil.Response, error) {
    return httputil.OK(map[string]string{
        "message": "Hello, " + r.Params.Name,
        "details": r.Data.Details,
    })
})
```

`Request[D, P]` embeds the original `*http.Request` and exposes the
`http.ResponseWriter` directly via `r.ResponseWriter`. When you write to the
writer yourself, return `httputil.NothingToHandle()` so the handler does not
also try to encode a response.

### Parameter Binding

Parameters are bound via the `param` struct tag, which supports a comma-separated
fallback list:

```go
type MyParams struct {
    // Look for "id" in the URL path.
    ID string `param:"path=id" validate:"required,uuid"`

    // Try query "filter", then header "X-Filter".
    Filter string `param:"query=filter,header=X-Filter"`

    // Try header "X-API-Key", fall back to a literal default.
    APIKey string `param:"header=X-API-Key,default=default-key"`

    // Query → header → default chain with validation.
    Version int `param:"query=v,header=X-Version,default=1" validate:"min=1"`
}
```

**Supported sources:**

- `path`: URL path parameters (e.g. `/users/{id}`).
- `query`: URL query string parameters (e.g. `?filter=active`).
- `header`: HTTP request headers (e.g. `X-API-Key: abc`).
- `default`: A static default value when no other source matches.

**Binding strategy:**

1. **First match wins** — sources are tried left-to-right; the first non-empty
   value is used.
2. **`default` is terminal** — once a default is applied, later sources are
   ignored.
3. **Defaults skip validation** — values populated from `default` are excluded
   from validation, so `default=0` and `validate:"min=1"` can coexist without
   surfacing a misleading error to the caller.
4. **Errors reflect the actual source** — if validation fails, the resulting
   problem details name the source key the value came from (e.g. `X-Filter`
   rather than `Filter`).

### Validation

Request bodies and parameters are validated with
[go-playground/validator](https://github.com/go-playground/validator). The
package registers a custom tag-name function that uses the `json` tag, falling
back to `form` for HTML form handlers:

```go
type CreateUserRequest struct {
    Name     string `json:"name" validate:"required,min=2,max=100"`
    Email    string `json:"email" validate:"required,email"`
    Age      int    `json:"age" validate:"required,min=18"`
    Password string `json:"password" validate:"required,min=8"`
}
```

For `NewHandler`, validation failures are automatically converted to RFC 9457
constraint-violation responses. For `NewFormHandler`, they are passed to your
action as `Request.Errors` instead.

### Transformers

If your `Request.Data`, `Request.Params`, or `Response.data` value implements
the `Transformer` interface, the handler will call `Transform(ctx)` after
decoding (for inputs) or before encoding (for outputs). This is useful for
normalising values, hydrating computed fields, or running tenancy checks.

```go
type CreateUser struct {
    Email string `json:"email"`
}

func (u *CreateUser) Transform(_ context.Context) error {
    u.Email = strings.ToLower(strings.TrimSpace(u.Email))
    return nil
}
```

Returning an error from `Transform` produces a 500 problem response and logs
the failure.

## Handler Options

`NewHandler` and `NewFormHandler` take any number of `HandlerOption`s:

| Option                | Default | Description                                                    |
| --------------------- | ------- | -------------------------------------------------------------- |
| `WithHandlerCodec`    | nil     | Codec used for request decoding and response encoding          |
| `WithHandlerGuard`    | nil     | Guard for request interception                                 |
| `WithHandlerLogger`   | nil     | Logger used by the handler                                     |
| `WithHandlerMessages` | nil     | Custom `MessageFunc` for validation messages (i18n)            |

When an option is omitted, the handler inherits codec and logger from the
`Server` it is registered on (resolved lazily on first request).

```go
handler := httputil.NewHandler(
    myHandlerFunc,
    httputil.WithHandlerCodec(httputil.NewHTMLServerCodec(tmpl)),
    httputil.WithHandlerGuard(myAuthGuard),
    httputil.WithHandlerLogger(logger),
)
```

## Form Handlers

`NewFormHandler` is a variant of `NewHandler` for HTML form workflows. Instead
of automatically writing an RFC 9457 error response when binding or validation
fails, it passes the errors to your action via `Request.Errors`, allowing you
to re-render the form with inline validation messages.

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
                return httputil.OK(httputil.Template{
                    Name: "users/new",
                    Data: FormPage{Data: r.Data, Errors: r.Errors},
                })
            }

            // Validation passed — process the form.
            return httputil.Redirect(http.StatusSeeOther, "/users")
        },
    ),
})
```

### BindErrors API

`BindErrors` aggregates validation and binding errors. Field keys use
dot-separated paths matching struct tag names (e.g. `address.city` for a nested
`City` field).

| Method       | Description                                                                                         |
| ------------ | --------------------------------------------------------------------------------------------------- |
| `HasAny()`   | Returns `true` if any data or parameter binding error occurred                                      |
| `Get(field)` | Returns the translated error message for a field (data errors take precedence)                      |
| `All()`      | Returns a flat map of all field-to-message errors (data errors take precedence on key collision)    |

`HasAny()` reports whether *any* error occurred, while `Get` and `All` only
contain entries for error types that can be mapped to individual fields. If
the underlying error cannot be mapped (for example a malformed request body),
`HasAny()` returns `true` while `Get` and `All` remain empty — inspect
`BindErrors.Data` or `BindErrors.Params` directly in that case.

### Custom Error Messages (i18n)

`WithHandlerMessages` provides a custom `MessageFunc` that controls
user-facing validation messages. It applies to body (Data) validation only —
parameter validation messages are produced by the binding pipeline.

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

httputil.NewHandler(action, messages)     // controls RFC 9457 messages
httputil.NewFormHandler(action, messages) // controls BindErrors.Get / All
```

## Response Helpers

The package provides constructors for common response shapes. Each returns a
`(*Response, error)` pair so they can be returned directly from an action.

```go
httputil.OK(data)       // 200
httputil.Created(data)  // 201
httputil.Accepted(data) // 202
httputil.NoContent()    // 204

httputil.Redirect(http.StatusSeeOther, "/new-location") // 3xx redirect
```

For arbitrary status codes use `NewResponse`:

```go
res := httputil.NewResponse(http.StatusPartialContent, data)
```

If you need to write directly to `r.ResponseWriter` (e.g. for streaming),
return `httputil.NothingToHandle()` so the handler will not attempt to
encode a response on top of what you wrote.

## HTML Templates

### HTMLServerCodec

`HTMLServerCodec` decodes form bodies (`application/x-www-form-urlencoded` and
`multipart/form-data` text fields) and renders responses through Go's
`html/template`. File uploads are *not* handled by the codec; use
`r.FormFile()` or `r.MultipartReader()` from the wrapped request directly.

To select which template is rendered, return an `httputil.Template{Name, Data}`
as the response data:

```go
return httputil.OK(httputil.Template{Name: "greeting", Data: r.Data})
```

`HTMLServerCodec` accepts any `TemplateExecutor`. Both `*template.Template`
and `*TemplateSet` satisfy this interface.

**HTML codec options:**

| Option                       | Default                    | Description                                                                                   |
| ---------------------------- | -------------------------- | --------------------------------------------------------------------------------------------- |
| `WithHTMLErrorTemplate`      | Minimal default error page | `*template.Template` for error pages (receives `*problem.DetailedError` as data)              |
| `WithHTMLFormDecoder`        | `go-playground/form`       | Custom `FormDecoder` for form data parsing                                                    |
| `WithHTMLMultipartMaxMemory` | 32 MB                      | Maximum memory used when parsing `multipart/form-data` forms                                  |

The error template always receives a `*problem.DetailedError` as its data,
giving access to `Title`, `Detail`, `Status`, `Type`, `Code`, `Instance`, and
`ExtensionMembers`. When the original error is not a `*problem.DetailedError`
one is constructed from the HTTP status code.

### TemplateSet for Page Isolation

When multiple pages define the same block names (e.g.
`{{ define "content" }}`), use `TemplateSet` to give each page its own
isolated copy of the base templates. Each entry in the set is cloned from the
shared base, so block definitions in one page do not conflict with another.

```go
base := template.Must(template.New("").Parse(""))
template.Must(base.New("layout").Parse(
    `<html><body>{{ block "content" . }}{{ end }}</body></html>`))
template.Must(base.New("error").Parse(
    `<html><body><h1>{{ .Title }}</h1><p>{{ .Detail }}</p></body></html>`))

ts, err := httputil.NewTemplateSet(base, map[string]string{
    "home":  `{{ template "layout" . }}{{ define "content" }}<h1>{{ .Title }}</h1>{{ end }}`,
    "about": `{{ template "layout" . }}{{ define "content" }}<p>{{ .Body }}</p>{{ end }}`,
})
if err != nil {
    log.Fatal(err)
}

codec := httputil.NewHTMLServerCodec(ts,
    httputil.WithHTMLErrorTemplate(ts.Lookup("error")),
)
```

`NewTemplateSet` returns `*TemplateConflictError` if a name in the templates
map collides with one already defined in the base.
`TemplateSet.ExecuteTemplate` returns `*TemplateUndefinedError` if a name is
not found.

### Loading Templates from Disk

In a real application, templates typically live on disk (or in an embedded
filesystem). The pattern is to parse shared layouts and partials into a base
template, then read page sources into a map for `NewTemplateSet`. Error pages
are just regular pages — they define their own blocks and use the shared
layout like any other page.

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

    if err := parseDir(base, fsys, "layouts"); err != nil {
        return nil, err
    }
    if err := parseDir(base, fsys, "partials"); err != nil {
        return nil, err
    }

    // Read page sources into a map. Each page gets its own clone of base.
    pages, err := readDir(fsys, "pages")
    if err != nil {
        return nil, err
    }

    return httputil.NewTemplateSet(base, pages)
}
```

A runnable version of this pattern, including the `parseDir` and `readDir`
helpers, is available as `ExampleNewTemplateSet_fromDisk` in the test suite.

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

## Error Handling

### RFC 9457 Problem Details

Error responses follow [RFC 9457](https://datatracker.ietf.org/doc/html/rfc9457) —
Problem Details for HTTP APIs:

```json
{
  "type": "https://github.com/nickbryan/httputil/blob/main/docs/problems/constraint-violation.md",
  "title": "Constraint Violation",
  "status": 422,
  "detail": "The request data violated one or more validation constraints",
  "code": "422-02",
  "instance": "/users",
  "violations": [
    { "detail": "should be a valid email", "pointer": "/email" }
  ]
}
```

`*problem.DetailedError` is the canonical type. It supports
`WithDetail(string)` and `WithExtension(key, value)` helpers that return a
clone with the field updated, leaving the original untouched.

The default error documentation URL is exposed as
`problem.ErrorDocumentationLocation` and can be overridden if you publish your
own problem-type documentation.

### Predefined Error Constructors

Constructors in the `problem` package take the originating `*http.Request` so
they can populate the `instance` field and method-specific text:

```go
// 400 Bad Request
problem.BadRequest(r)
problem.BadRequest(r).WithDetail("payload could not be parsed")

// 400 Bad Parameters (binding failures)
problem.BadParameters(r,
    problem.Parameter{Parameter: "id", Detail: "must be a UUID", Type: problem.ParameterTypePath},
)

// 401 / 403 / 404 / 409
problem.Unauthorized(r)
problem.Forbidden(r)
problem.NotFound(r)
problem.ResourceExists(r)

// 422 Unprocessable Entity (constraint and rule violations)
problem.ConstraintViolation(r,
    problem.Property{Detail: "must be a valid email", Pointer: "/email"},
)
problem.BusinessRuleViolation(r,
    problem.Property{Detail: "balance cannot go negative", Pointer: "/amount"},
)

// 500 Internal Server Error
problem.ServerError(r)
```

Returning a `*problem.DetailedError` from an action causes the codec's
`EncodeError` to render it with the appropriate status and content type
(`application/problem+json` for the JSON codec, an HTML error page for the
HTML codec).

## Middleware

### Built-in Middleware

The server applies two middlewares automatically, in this order:

1. **Panic recovery** — recovers from panics in handlers, logs the panic with
   stack trace, and writes a 500 response (if no body has been written yet).
2. **Max body size** — short-circuits requests whose `Content-Length` exceeds
   `WithServerMaxBodySize` with a 413 response, and wraps the request body
   with `http.MaxBytesReader` to enforce the limit during reads.

### Custom Middleware

Custom middleware uses the `MiddlewareFunc` type:

```go
func loggingMiddleware(logger *slog.Logger) httputil.MiddlewareFunc {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            next.ServeHTTP(w, r)
            logger.InfoContext(r.Context(), "Request processed",
                slog.String("method", r.Method),
                slog.String("path", r.URL.Path),
                slog.Duration("duration", time.Since(start)),
            )
        })
    }
}
```

Apply middleware to endpoints via `EndpointGroup.WithMiddleware`:

```go
endpoints := httputil.EndpointGroup{
    httputil.Endpoint{Method: http.MethodGet, Path: "/users", Handler: httputil.NewHandler(listUsers)},
    httputil.Endpoint{Method: http.MethodPost, Path: "/users", Handler: httputil.NewHandler(createUser)},
}

server.Register(endpoints.WithMiddleware(loggingMiddleware(logger))...)

// Apply several middlewares in one call. They run in the order given:
// loggingMiddleware first, then authMiddleware, then the handler.
server.Register(endpoints.WithMiddleware(
    loggingMiddleware(logger),
    authMiddleware(authenticator),
)...)
```

Within a single `WithMiddleware` call, middlewares run in the order given
(first arg runs first). Across chained calls, the most recent call wraps the
previous one, so it runs first — this lets you compose nested groups so an
outer group's middleware wraps everything from inner groups:

```go
admin := adminEndpoints.WithMiddleware(authMiddleware(authenticator)) // auth on admin only
all := append(httputil.EndpointGroup{}, admin...)
all = append(all, publicEndpoints...)

// loggingMiddleware wraps everything; authMiddleware still only runs on admin endpoints.
// On admin requests: log -> auth -> handler. On public requests: log -> handler.
server.Register(all.WithMiddleware(loggingMiddleware(logger))...)
```

This LIFO across-call ordering is intentionally different from
`WithClientInterceptor`, which uses FIFO across calls because client
interceptors form a flat chain rather than a nested composition.

## Guards

Guards intercept and optionally rewrite a request before it reaches the
action. They run *after* routing but *before* body decoding and parameter
binding, making them ideal for authentication, authorisation, API key
validation, or attaching values to the request context.

### Request Interception

A guard implements the `Guard` interface:

```go
type AuthGuard struct {
    secretKey string
}

func (g *AuthGuard) Guard(r *http.Request) (*http.Request, error) {
    token := r.Header.Get("Authorization")
    if token == "" {
        return nil, problem.Unauthorized(r)
    }

    // ... validate token, build user info ...

    return r.WithContext(context.WithValue(r.Context(), userKey{}, userInfo)), nil
}
```

Returning a non-nil request swaps it in for downstream handling; returning a
non-nil error short-circuits the request. If the error is a
`*problem.DetailedError`, it is rendered through the codec; otherwise a 500
problem response is written and the underlying error is logged.

For one-off guards there is also `httputil.GuardFunc`:

```go
authGuard := httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
    if r.Header.Get("Authorization") == "" {
        return nil, problem.Unauthorized(r)
    }
    return r, nil
})
```

Apply a guard at the endpoint level with `NewEndpointWithGuard`:

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

Or scoped to a handler with `WithHandlerGuard`. Guards configured via
`EndpointGroup.WithGuard` apply to the entire group.

### Guard Stacks

`GuardStack` runs multiple guards in order, threading the (possibly modified)
request through each one. Iteration stops at the first error.

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

`EndpointGroup` is a slice of `Endpoint` with helpers for shared concerns. All
helpers return new groups; the originals are not modified.

```go
userEndpoints := httputil.EndpointGroup{
    httputil.Endpoint{Method: http.MethodGet, Path: "/users", Handler: httputil.NewHandler(listUsers)},
    httputil.Endpoint{Method: http.MethodPost, Path: "/users", Handler: httputil.NewHandler(createUser)},
}

secured := userEndpoints.
    WithPrefix("/api/v1").
    WithMiddleware(loggingMiddleware(logger)).
    WithGuard(&RateLimitGuard{})

server.Register(secured...)
```

`WithGuard` composes new guards onto a `GuardStack` so multiple calls compose.

## Wrapping Standard `http.Handler`

`WrapNetHTTPHandler` and `WrapNetHTTPHandlerFunc` adapt a stock
`http.Handler` so it can be registered on the server and pick up guards. The
wrapper participates in guard execution but does not perform body decoding or
response encoding — those remain the handler's responsibility.

```go
server.Register(httputil.Endpoint{
    Method: http.MethodGet,
    Path:   "/healthz",
    Handler: httputil.WrapNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        _, _ = w.Write([]byte("ok"))
    }),
})
```

Guard failures from a wrapped `http.Handler` are written as
`application/problem+text` with the `*problem.DetailedError` rendered through
its `Error()` string.

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

    server.Register(httputil.Endpoint{
        Method: http.MethodGet,
        Path:   "/greetings",
        Handler: httputil.NewHandler(
            func(_ httputil.RequestEmpty) (*httputil.Response, error) {
                return httputil.OK([]string{"Hello, World!", "Hola Mundo!"})
            },
        ),
    })

    server.Serve(context.Background())

    // curl localhost:8080/greetings
    // ["Hello, World!","Hola Mundo!"]
}
```

### JSON Handler with Request/Response

```go
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

// curl -iS -X POST -H "Content-Type: application/json" -d '{"name":"Nick"}' localhost:8080/greetings
// HTTP/1.1 201 Created
// Content-Type: application/json; charset=utf-8
//
// {"message":"Hello Nick!"}
```

### JSON Handler with Path Parameters

```go
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

// curl localhost:8080/greetings/Nick
// ["Hello, Nick!","Hola Nick!"]
```

### Combined Data and Parameters

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
            return httputil.OK(map[string]string{
                "id":    r.Params.ID,
                "name":  r.Data.Name,
                "email": r.Data.Email,
            })
        }),
    }
}
```

### Basic net/http Handler

```go
server.Register(httputil.Endpoint{
    Method: http.MethodGet,
    Path:   "/greetings",
    Handler: httputil.WrapNetHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        _, _ = w.Write([]byte(`["Hello, World!","Hola Mundo!"]`))
    }),
})
```

### HTML Handler with Form Data

Use `HTMLServerCodec` to build endpoints that accept form submissions and
render HTML templates. This is ideal for HTMX-powered or traditional
server-rendered web applications.

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
        ),
    }
}
```

### Custom Middleware and Guards

```go
func setupServer(logger *slog.Logger) *httputil.Server {
    server := httputil.NewServer(logger)

    loggingMiddleware := func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            logger.InfoContext(r.Context(), "Request started",
                slog.String("method", r.Method),
                slog.String("path", r.URL.Path))
            next.ServeHTTP(w, r)
        })
    }

    authGuard := httputil.GuardFunc(func(r *http.Request) (*http.Request, error) {
        if r.Header.Get("Authorization") == "" {
            return nil, problem.Unauthorized(r)
        }
        return r, nil
    })

    endpoints := httputil.EndpointGroup{
        httputil.Endpoint{Method: http.MethodGet, Path: "/users", Handler: httputil.NewHandler(listUsers)},
        httputil.Endpoint{Method: http.MethodPost, Path: "/users", Handler: httputil.NewHandler(createUser)},
    }

    server.Register(endpoints.
        WithMiddleware(loggingMiddleware).
        WithGuard(authGuard).
        WithPrefix("/api/v1")...)

    return server
}
```

## Client

`httputil.Client` is a thin, opinionated wrapper around `*net/http.Client`
designed for calling JSON (and other RFC-9457-aware) services. Most
interactions go through the package-level generic helpers
`Get[T]`, `Post[T]`, `Put[T]`, `Patch[T]`, and `Delete[T]`, which:

- Build the request (URL joining, query merging, headers).
- Encode the request body using the configured `ClientCodec`.
- Send the request through the interceptor chain.
- For 2xx responses, decode the body into `T` and return a `*Result[T]`.
- For non-2xx responses, return a typed error (`*ProblemResponseError`,
  `*UnexpectedResponseError`, or whatever a custom `ErrorDecoder` produces).
- Drain and close the response body — callers do **not** need to call
  `resp.Body.Close()`.

### Creating a Client

```go
client := httputil.NewClient(
    logger,
    httputil.WithClientBasePath("https://api.example.com"),
    httputil.WithClientTimeout(10 * time.Second),
    httputil.WithClientInterceptor(NewLogInterceptor(logger)),
)
```

`NewClient` requires a `*slog.Logger` so internal failures (response body
drain/close errors, etc.) can be reported. The default `ClientCodec` is JSON
(`application/json; charset=utf-8`).

### Making Requests

```go
type User struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

result, err := httputil.Get[User](ctx, client, "/users/123",
    httputil.WithRequestHeader("Authorization", "Bearer "+token),
    httputil.WithRequestParam("expand", "profile"),
)
if err != nil {
    return nil, fmt.Errorf("getting user: %w", err)
}

user := result.Data            // typed body
status := result.StatusCode    // *http.Response is embedded
```

```go
// POST /users
created, err := httputil.Post[User](ctx, client, "/users", User{Name: "Alice"})

// PUT, PATCH, DELETE follow the same shape:
_, err = httputil.Put[User](ctx, client, "/users/123", User{Name: "Alice"})
_, err = httputil.Patch[User](ctx, client, "/users/123", patch)
_, err = httputil.Delete[struct{}](ctx, client, "/users/123", nil)
```

The configured codec is used for both encoding the request body and decoding
the response body. The `Content-Type` and `Accept` headers default to the
codec's content type and can be overridden with `WithRequestHeader`.

### The `Result[T]` Return Value

```go
type Result[T any] struct {
    *http.Response
    Data T
}
```

`*Result[T]` embeds the original `*http.Response`, so status code, headers,
trailers, and other response metadata are still accessible. The body is
**already drained and closed** by the time the result is returned, do not
attempt to read from `Result.Body`.

### Skipping Response Decoding

For requests where you only care about the status (204 endpoints, fire-and-
forget calls, custom decoding paths), use `struct{}` as the type parameter.
The body will be drained and closed but not decoded, even if the server sent
one:

```go
result, err := httputil.Delete[struct{}](ctx, client, "/users/123", nil)
if err != nil {
    return err
}
_ = result.StatusCode
```

### Sending Raw Bodies

If you pass an `io.Reader` as the body to `Post`/`Put`/`Patch`/`Delete`, it is
sent verbatim, the codec is **not** invoked. The default `Content-Type` is
still set to the codec's content type, so set it explicitly with
`WithRequestHeader` when sending non-JSON payloads:

```go
_, err := httputil.Post[struct{}](ctx, client, "/upload",
    bytes.NewReader(rawBytes),
    httputil.WithRequestHeader("Content-Type", "application/octet-stream"),
)
```

### Headers and Query Parameters

Per-request options merge with anything already encoded in the path:

```go
result, err := httputil.Get[Page](ctx, client, "/search?tag=a",
    httputil.WithRequestParam("tag", "b"),                      // ?tag=a&tag=b
    httputil.WithRequestParams(url.Values{"page": {"1"}}),       // adds page=1
    httputil.WithRequestHeader("Accept-Language", "en-GB"),
    httputil.WithRequestHeaders(http.Header{"X-Trace": {"abc"}}),
)
```

When a query key appears in both the path and the request options, all values
are preserved (the values are appended, not replaced).

### Client Error Handling

Anything outside the 2xx range produces an error. By default:

- A response with `Content-Type: application/problem+...` is decoded into a
  `*problem.DetailedError` and returned wrapped in `*ProblemResponseError`.
- Any other non-2xx response returns `*UnexpectedResponseError` with the
  original `*http.Response` attached (body already drained).

`*ProblemResponseError` unwraps to its `*problem.DetailedError`, so a single
`errors.AsType` call can extract the structured detail regardless of the
wrapper:

```go
result, err := httputil.Get[User](ctx, client, "/users/"+id)
if err != nil {
    if pd, ok := errors.AsType[*problem.DetailedError](err); ok {
        switch pd.Code {
        case "404-01":
            return nil, ErrUserNotFound
        case "429-01":
            return nil, ErrRateLimited
        }
        return nil, fmt.Errorf("api problem: %w", pd)
    }

    var unexpected *httputil.UnexpectedResponseError
    if errors.As(err, &unexpected) {
        return nil, fmt.Errorf("unexpected status %d", unexpected.Response.StatusCode)
    }

    return nil, fmt.Errorf("calling api: %w", err)
}
return &result.Data, nil
```

`httputil.IsProblem(resp)` exposes the same `application/problem+*`
content-type check used internally; it is occasionally useful when you handle
responses manually via `Client.Do`.

### Custom Error Decoders

Some APIs return non-RFC-9457 error bodies. `WithClientErrorDecoder` lets you
plug in a function that receives the raw response and the configured codec
and returns the appropriate error type. The decoder runs on every non-2xx
response.

```go
type APIError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

client := httputil.NewClient(logger,
    httputil.WithClientBasePath("https://api.example.com"),
    httputil.WithClientErrorDecoder(httputil.DecodeErrorAs[*APIError]()),
)

_, err := httputil.Get[User](ctx, client, "/users/123")

var apiErr *APIError
if errors.As(err, &apiErr) {
    // handle structured API error
}
```

`DecodeErrorAs[T]()` is a generic helper for the common case where the body
maps directly to a single error type. Returning `nil` from a custom decoder
falls back to `*UnexpectedResponseError`. Use `WithRequestErrorDecoder` to
override the decoder for a single call (helpful for endpoints with a
different error envelope).

### Interceptors (Client Middleware)

Interceptors wrap the client's `http.RoundTripper` and let you add behaviour
around every request — logging, retries, tracing, auth headers, metrics, and
so on — without touching call sites:

```go
type InterceptorFunc func(next http.RoundTripper) http.RoundTripper
```

Each interceptor receives the next round-tripper in the chain and returns a
new one. Within a single `WithClientInterceptor` call, interceptors run in the
order given. Across multiple calls, earlier-added interceptors run first
(FIFO). This differs from `EndpointGroup.WithMiddleware`, which uses LIFO
across calls because endpoint middleware participates in nested-group
composition; client interceptors are a single flat chain on one client, so
listing them in invocation order reads more naturally.

Guidelines:

- Keep interceptors small and focused — one responsibility per interceptor.
- Don't mutate the incoming `*http.Request` in place. Use `req.Clone(...)` or
  `req.WithContext(...)` when you need to modify it.
- Always call `next.RoundTrip(req)` unless you intentionally short-circuit
  (for example, returning a cached response or an early error).
- Be careful with retries: bodies passed as `io.Reader` are not generally
  replayable. Buffer the body or use `http.Request.GetBody`-friendly readers
  if you need to retry.

**Example: a simple logging interceptor.**

```go
func NewLogInterceptor(logger *slog.Logger) httputil.InterceptorFunc {
    return func(next http.RoundTripper) http.RoundTripper {
        return httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
            start := time.Now()
            logger.DebugContext(req.Context(), "Client request started",
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
            logger.LogAttrs(req.Context(), slog.LevelInfo, "Client request completed", attrs...)

            return resp, err
        })
    }
}
```

### Escape Hatch: `Client.Do`

`Client.Do(req *http.Request) (*http.Response, error)` mirrors the
`http.Client.Do` signature for cases the generic helpers don't cover —
streaming bodies you need to read incrementally, server-sent events,
unusual status-code handling, manual content negotiation, libraries that
expect a "doer" with a `Do` method, and so on. Interceptors still apply
because `Do` goes through the same transport. The base path is **not**
prepended; the caller owns the full URL.

```go
req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
    "https://api.example.com/events", nil)

resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close() // when calling Do directly, you own the body
// ... stream resp.Body ...
```

### Client Options

| Option                     | Default                 | Description                                                        |
|----------------------------|-------------------------|--------------------------------------------------------------------|
| `WithClientBasePath`       | `""`                    | Base URL prefixed to relative request paths                        |
| `WithClientCodec`          | JSON                    | `ClientCodec` used for request and response bodies                 |
| `WithClientCookieJar`      | nil                     | `http.CookieJar` for the client                                    |
| `WithClientErrorDecoder`   | nil                     | `ErrorDecoder` invoked for non-2xx responses                       |
| `WithClientInterceptor`    | none                    | Wraps the base transport to provide client middleware              |
| `WithClientRedirectPolicy` | nil                     | Custom redirect policy                                             |
| `WithClientTimeout`        | 60s                     | Total request timeout                                              |
| `WithClientTransport`      | `http.DefaultTransport` | Base transport that interceptors wrap                              |

### Request Options

| Option                     | Description                                                          |
|----------------------------|----------------------------------------------------------------------|
| `WithRequestErrorDecoder`  | Override the error decoder for a single request                      |
| `WithRequestHeader`        | Add a single header                                                  |
| `WithRequestHeaders`       | Add multiple headers from an `http.Header`                           |
| `WithRequestParam`         | Add a single query parameter                                         |
| `WithRequestParams`        | Add multiple query parameters from a `url.Values`                    |

## Design Choices

### RFC 9457 Problem Details

Error responses follow [RFC 9457](https://datatracker.ietf.org/doc/html/rfc9457)
to give clients consistent, machine-readable problem information including a
stable `type` URI, a human-readable `title`, and structured violation
extensions where appropriate.

### Type Safety with Generics

Both the handler and client APIs use Go generics to keep request data,
parameters, and response payloads strongly typed end-to-end. This eliminates
ad-hoc `interface{}` boilerplate and makes the contract for each endpoint or
client call self-documenting.

### Single-Tag Parameter Binding

The `param` tag is a single source of truth for path/query/header/default
fallback chains. Validation errors are reported against the source the value
actually came from, so clients see the correct parameter name in the
response.

### Codec-Driven Encoding

A single `ServerCodec` interface drives request decoding and response (and
error) encoding for the server, while `ClientCodec` drives the same for the
client. JSON is the default; the package ships an HTML implementation for
form/template workflows. Custom codecs (XML, MessagePack, CBOR, etc.) are a
matter of implementing the interface.

### Middleware vs. Interceptor Ordering

Server middleware (`EndpointGroup.WithMiddleware`) uses LIFO across calls so
that nested groups compose naturally — outer groups wrap inner ones. Client
interceptors (`WithClientInterceptor`) form a single flat chain on one
client, so they use FIFO across calls — the order you add them is the order
they run. Both behaviours are documented in `MiddlewareFunc`/`InterceptorFunc`.

### Graceful Shutdown

`Server.Serve` listens for `SIGINT`, `SIGTERM`, and `SIGQUIT` and triggers a
graceful shutdown bounded by `WithServerShutdownTimeout`. The shutdown uses a
fresh context, so an already-cancelled parent context does not immediately
abort in-flight requests.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository.
2. Create your feature branch (`git checkout -b feature/amazing-feature`).
3. Commit your changes (`git commit -m 'Add some amazing feature'`).
4. Push to the branch (`git push origin feature/amazing-feature`).
5. Open a Pull Request.

Local development uses the Make targets in `Makefile`:

```
make test       # run tests with race detection
make lint-fix   # run golangci-lint with auto-fix
```

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE)
file for details.
