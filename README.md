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
package main

import (
  "context"
  "log/slog"
  "net/http"

  "github.com/nickbryan/slogutil"

  "github.com/nickbryan/httputil"
  "github.com/nickbryan/httputil/problem"
)

type authGuard struct{}

func newAuthGuard() authGuard { return authGuard{} }

func (g authGuard) Guard(r *http.Request) (*httputil.Response, error) {
  return nil, problem.Unauthorized(r)
}

func newTestHandler(l *slog.Logger) httputil.Handler {
  type params struct {
    Test string `query:"test"`
  }

  type response struct {
    Value string `json:"value"`
  }

  return httputil.NewJSONHandler(func(r httputil.RequestParams[params]) (*httputil.Response, error) {
    l.Info("written")
    return httputil.OK(response{Value: r.Params.Test})
  })
}

func endpoints(l *slog.Logger) httputil.EndpointGroup {
  return httputil.EndpointGroup{
    httputil.ProtectEndpoint(httputil.Endpoint{
      Method:  http.MethodGet,
      Path:    "/balue",
      Handler: newTestHandler(l),
    }, newAuthGuard()),
    httputil.ProtectEndpoint(httputil.Endpoint{
      Method:  http.MethodPost,
      Path:    "/test",
      Handler: newTestHandler(l),
    }, newAuthGuard()),
  }
}

func main() {
  l := slogutil.NewJSONLogger()
  server := httputil.NewServer(l)

  server.Register(endpoints(l).WithPrefix("/api").WithGuard(newAuthGuard())...)
  server.Serve(context.Background())
}

```

## TODO
* [ ] Look at the parameter decoding code and finish it up properly.
* [ ] Update handler code to return the correct problems.
* [ ] Finish testing the existing code to achieve sensible coverage.
* [ ] Add common middleware.
* [ ] How do we allow people to return a custom error payload if required so they are not locked to problem json?
* [ ] Implement proper JSON pointer handling on validation errors as per https://datatracker.ietf.org/doc/html/rfc6901.
* [ ] Decide on how to wrap logger, implement and test - use as is or clone the writeHandler so we can provide a static message and add the error as an attribute? Would also allow us to set pc?
* [ ] Write the client side.
* [ ] Finalise all default values, ensure they are correct.
* [ ] This README needs filling out properly
* [ ] Update README to highlight problem json as a feature and provide examples of usage.
* [ ] Document how errors take priority over responses, if an error is returned no response will be written if one is also returned. y.
* [ ] Check compatibility with Orchestrion.
* [ ] Finalise all package documentation.
