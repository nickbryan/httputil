# httputil
Package `httputil` provides utility helpers for working with net/http adding sensible defaults, bootstrapping, and 
removing boilerplate code required to build web services.

<div align="center">

[![Test](https://github.com/nickbryan/httputil/actions/workflows/test.yml/badge.svg)](https://github.com/nickbryan/httputil/actions)
[![Coverage](https://raw.githubusercontent.com/nickbryan/httputil/badges/.badges/main/coverage.svg)](https://github.com/nickbryan/httputil/actions)
[![Go Report Card](https://goreportcard.com/badge/nickbryan/httputil)](https://goreportcard.com/report/nickbryan/httputil)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/nickbryan/httputil/blob/master/LICENSE)

</div>

## TODO
* [ ] Should we use problem details as the error format? If so, do we have a problem package or just have them as Error in the root?
* [ ] Move Details to httputil.ProblemDetails and then consider a package problem that just creates instances of ProblemDetails. What to do about handler? Should it still return error or only the ProblemDetails type? How could we also drop the need to pass a pointer for nil in the return arguments? Something instead of nil to represent empty? Union type? Httputil.NoProblem
* [ ] Finish tests on server.go. 
* [ ] Finish testing handler.go.
* [ ] Check over status codes used and error messages sent to user and logs are correct in the JSON handler code.
* [ ] Write basic usage documentation and feature list.
* [ ] Export hendlerError type and drop new function?
* [ ] Test the Endpoint wrapper functions.
* [ ] Decide on how to wrap logger, implement and test - use as is or clone the writeHandler so we can provide a static message and add the error as an attribute? Would also allow us to set pc?
* [ ] Figure out how to handle query params and path params for validation and decoding.
* [ ] Fix lint issues. 
* [ ] Ensure that panic middleware is correct, what comes back from recover and shoudl each type be handled (maybe just err and string).
* [ ] Add common middleware.
* [ ] Test all middleware.
* [ ] Finalise all default values, ensure they are correct. 
* [ ] This README needs filling out properly.
* [ ] Finalise all package documentation.
