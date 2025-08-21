package httputil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/nickbryan/httputil/problem"
)

type ClientCodec interface {
	ContentType() string
	Encode(data any) (io.Reader, error)
	Decode(r io.Reader, into any) error
}

// JSONClientCodec provides methods to encode data as JSON or decode data from JSON in
// HTTP requests and responses.
type JSONClientCodec struct{}

// NewJSONClientCodec creates a new JSONClientCodec instance.
func NewJSONClientCodec() JSONClientCodec {
	return JSONClientCodec{}
}

// ContentType returns the Content-Type header value for JSON requests and
// responses.
func (c JSONClientCodec) ContentType() string {
	return "application/json; charset=utf-8"
}

// Encode encodes the given data into a new io.Reader.
func (c JSONClientCodec) Encode(data any) (io.Reader, error) {
	if data == nil {
		return nil, nil
	}

	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encoding request body as JSON: %w", err)
	}

	return bytes.NewReader(b), nil
}

// Decode reads and decodes the JSON body of an HTTP response into the provided
// target struct or variable.
func (c JSONClientCodec) Decode(r io.Reader, into any) error {
	if err := json.NewDecoder(r).Decode(into); err != nil {
		return fmt.Errorf("decoding response body as JSON: %w", err)
	}

	return nil
}

// ServerCodec is an interface for encoding and decoding HTTP requests and responses.
// It provides methods for decoding request data and encoding response data or
// errors.
type ServerCodec interface {
	// Decode decodes the request data and sets it on into. Implementations of
	// Decode should return [io.EOF] if the request data is empty when Decode is
	// called.
	Decode(r *http.Request, into any) error
	// Encode writes the given data to the http.ResponseWriter after encoding it,
	// returning an error if encoding fails.
	Encode(w http.ResponseWriter, data any) error
	// EncodeError encodes the provided error into the HTTP response writer and
	// returns an error if encoding fails.
	EncodeError(w http.ResponseWriter, err error) error
}

// JSONServerCodec provides methods to encode data as JSON or decode data from JSON in
// HTTP requests and responses.
type JSONServerCodec struct{}

// NewJSONServerCodec creates a new JSONServerCodec instance.
func NewJSONServerCodec() JSONServerCodec {
	return JSONServerCodec{}
}

// Decode reads and decodes the JSON body of an HTTP request into the provided
// target struct or variable. Returns an error if decoding fails or if the
// request body is nil.
func (c JSONServerCodec) Decode(r *http.Request, into any) error {
	if r.Body == nil {
		return nil
	}

	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		return fmt.Errorf("decoding request body as JSON: %w", err)
	}

	return nil
}

// Encode writes the given data as JSON to the provided HTTP response writer
// with the appropriate Content-Type header.
func (c JSONServerCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	return writeJSON(w, data)
}

// EncodeError encodes an error into an HTTP response, handling
// `problem.DetailedError` if applicable to set the correct content type, or
// falling back to standard JSON encoding otherwise.
func (c JSONServerCodec) EncodeError(w http.ResponseWriter, err error) error {
	var problemDetails *problem.DetailedError
	if errors.As(err, &problemDetails) {
		w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
		return writeJSON(w, problemDetails)
	}

	return c.Encode(w, err)
}

// writeJSON writes the given data as JSON to the provided writer. It returns an
// error if encoding fails.
func writeJSON(w io.Writer, data any) error {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return fmt.Errorf("encoding response data as JSON: %w", err)
	}

	return nil
}
