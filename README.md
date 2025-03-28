# httputil
Package `httputil` provides utility helpers for working with net/http adding sensible defaults, bootstrapping, and 
removing boilerplate code required to build web services.

<div align="center">

[![Test](https://github.com/nickbryan/httputil/actions/workflows/test.yml/badge.svg)](https://github.com/nickbryan/httputil/actions)
[![Coverage](https://raw.githubusercontent.com/nickbryan/httputil/badges/.badges/main/coverage.svg)](https://github.com/nickbryan/httputil/actions)
[![Go Report Card](https://goreportcard.com/badge/nickbryan/httputil)](https://goreportcard.com/report/nickbryan/httputil)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/nickbryan/httputil/blob/master/LICENSE)

</div>

## Features
* **SerDe Handlers:** Serialize/Deserialize request and response data with decreased boilerplate code.
  * `NewJSONHandler` provides encoding/decoding for JSON-based handlers.
* **Reduced Error Handling:** Common error scenarios are handled and logged for consistency.
  * Server will attempt graceful shutdown and log errors appropriately.
* **Problem JSON Implementation:** Standardized problem details for error responses as per RFC 9457.
  * Supports customization of problem payloads and proper JSON pointer handling for validation errors.
* **Middleware Support:** Add common middleware such as panic recovery and logging with minimal effort.
* **Endpoint Management:** Easily register and organise endpoints with prefixing and middleware chaining.

## Quick Start
```go
// Package main is a test example.
package main

import (
  "context"
  "fmt"
  "log/slog"
  "net/http"

  "github.com/nickbryan/slogutil"

  "github.com/nickbryan/httputil"
  "github.com/nickbryan/httputil/problem"
)

func newAuthInterceptor() httputil.RequestInterceptorFunc {
  return func(r *http.Request) (*http.Request, error) {
    params := struct {
      Token string `header:"Bearer" validate:"required"`
    }{}
    if err := httputil.BindValidParameters(r, &params); err != nil {
      return nil, fmt.Errorf("binding parameters: %w", err)
    }

    if params.Token == "valid" {
      return r.WithContext(context.WithValue(r.Context(), "user", "123")), nil
    }

    return nil, problem.Unauthorized(r)
  }
}

func newTestHandler(l *slog.Logger) httputil.Handler {
  type params struct {
    Test string `query:"test"`
  }

  type response struct {
    Value string `json:"value"`
  }

  return httputil.NewJSONHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
    l.InfoContext(r.Context(), "written")
    return httputil.OK(response{Value: r.Params.Test})
  })
}

func endpoints(l *slog.Logger) httputil.EndpointGroup {
  return httputil.EndpointGroup{
    {
      Method:  http.MethodPost,
      Path:    "/login",
      Handler: newTestHandler(l),
    },
    httputil.NewEndpointWithRequestInterceptor(httputil.Endpoint{
      Method:  http.MethodPost,
      Path:    "/test",
      Handler: newTestHandler(l),
    }, newAuthInterceptor()),
  }
}

func main() {
  l := slogutil.NewJSONLogger()
  server := httputil.NewServer(l)

  server.Register(endpoints(l).WithPrefix("/api")...)
  server.Serve(context.Background())
}
```

## TODO
* [ ] Implement proper JSON pointer handling on validation errors as per https://datatracker.ietf.org/doc/html/rfc6901.
* [ ] Write the client side.
* [ ] This README needs filling out properly
* [ ] Document how errors take priority over responses, if an error is returned no response will be written if one is also returned. y.
* [ ] Do I move internal/testutil to its own package too?
