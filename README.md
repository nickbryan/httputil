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
    * `NewJSONHandler` provides encoding/decoding for JSON based handlers.
* **Reduced Error Handling:** Common error scenarios are handled and logged for consistency.
    * Server will attempt graceful shutdown and log errors appropriately.
* ...

## Quick Start
```go
package main

import (
	"context"
	"net/http"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"
)

func main() {
	logger := slogutil.NewJSONLogger()
	server := httputil.NewServer(logger)

	server.Register(
		httputil.EndpointsWithPrefix(
			"/api",
			httputil.EndpointsWithMiddleware(
				httputil.NewPanicRecoveryMiddleware(logger),
				newTestEndpoint(),
			)...,
		)...,
	)

	// GET http://localhost:8080/api/names
	server.Serve(context.Background())
}

func newTestEndpoint() httputil.Endpoint {
	return httputil.Endpoint{
		Method:  http.MethodGet,
		Path:    "/names",
		Handler: newTestHandler(),
	}
}

func newTestHandler() http.Handler {
	type response struct {
		Names []string `json:"names"`
	}

	return httputil.NewJSONHandler(func(r httputil.Request[struct{}]) (*httputil.Response[response], error) {
		return httputil.NewResponse(http.StatusOK, response{Names: []string{"Dr Jones"}}), nil
	})
}
```

## TODO
* [ ] Implement the remaining problem details for common errors.
* [ ] Update README to highlight problem json as a feature and provide examples of usage.
* [ ] How do we allow people to return a custom error payload if required so they are not locked to  problem json?
* [ ] Should each problem.Details have a code field so that we can increment them per business violation rule? So "422-NS-1" or "422-01"
* [ ] Document how errors take priority over responses, if an error is returned no response will be written if one is also returned. 
* [ ] Implement proper JSON pointer handling on validation errors as per https://datatracker.ietf.org/doc/html/rfc6901.
* [ ] Prevent overwriting of base values in the problem json marshaling code.
* [ ] What to do about handler? Should it still return error or only the ProblemDetails type? How could we also drop the need to pass a pointer for nil in the return arguments? Something instead of nil to represent empty? Union type? Httputil.NoProblem
* [ ] Finish test existing code to achieve sensible coverage.
* [ ] Check over status codes used and error messages sent to user and logs are correct in the JSON handler code.
* [ ] Decide on how to wrap logger, implement and test - use as is or clone the writeHandler so we can provide a static message and add the error as an attribute? Would also allow us to set pc?
* [ ] Figure out how to handle query params and path params for validation and decoding.
* [ ] Ensure that panic middleware is correct, what comes back from recover and shoudl each type be handled (maybe just err and string).
* [ ] Add common middleware.
* [ ] Finalise all default values, ensure they are correct. 
* [ ] This README needs filling out properly.
* [ ] Finalise all package documentation.
