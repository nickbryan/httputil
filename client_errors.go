package httputil

import (
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"strings"

	"github.com/nickbryan/httputil/problem"
)

// ErrorDecoder is a function that decodes an error response into a Go error.
// It receives the response and the client's codec so it can decode the body.
// A nil return falls back to UnexpectedResponseError.
type ErrorDecoder func(resp *http.Response, codec ClientCodec) error

// IsProblem reports whether the response has an RFC 9457 problem detail
// Content-Type (e.g. application/problem+json, application/problem+xml).
// It returns false if the Content-Type header is missing or malformed, and
// ignores media type parameters such as charset.
func IsProblem(resp *http.Response) bool {
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return strings.HasPrefix(mt, "application/problem+")
}

// ProblemResponseError is returned when the server responds with an
// application/problem+ error body. Both Response and Problem are always
// non-nil when returned by the Client. The response body is read and closed
// before the error is returned.
type ProblemResponseError struct {
	Response *http.Response
	Problem  *problem.DetailedError
}

func (e *ProblemResponseError) Error() string {
	return "problem response: " + e.Problem.Error()
}

func (e *ProblemResponseError) Unwrap() error {
	return e.Problem
}

// UnexpectedResponseError is returned when the server responds with a
// non-success status code that is not a problem detail. Response is always
// non-nil when returned by the Client. The response body is read and closed
// before the error is returned; use ErrorDecoder to inspect the body.
type UnexpectedResponseError struct {
	Response *http.Response
}

func (e *UnexpectedResponseError) Error() string {
	return fmt.Sprintf("unexpected response status: %d", e.Response.StatusCode)
}

// DecodeErrorAs returns an ErrorDecoder that decodes the response body into the
// error type T. When decoding succeeds, the resulting T is returned as the
// error. When decoding fails, the decode error is returned directly.
func DecodeErrorAs[T error]() ErrorDecoder {
	return func(resp *http.Response, codec ClientCodec) error {
		var target T
		if err := codec.Decode(resp.Body, &target); err != nil {
			return fmt.Errorf("decoding error response: %w", err)
		}

		// When T is a pointer type (e.g. *APIError) and the body decodes to
		// nil (e.g. JSON null), target is a nil pointer. Returning it as the
		// error interface would create a non-nil interface with a nil
		// underlying value: err != nil passes but err.Error() panics. Return
		// UnexpectedResponseError instead to avoid this.
		if val := reflect.ValueOf(&target).Elem(); val.Kind() == reflect.Pointer && val.IsNil() {
			return &UnexpectedResponseError{Response: resp}
		}

		return target
	}
}
