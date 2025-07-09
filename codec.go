package httputil

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/go-playground/form"

	"github.com/nickbryan/httputil/problem"
)

// Codec is an interface for encoding and decoding HTTP requests and responses.
// It provides methods for decoding request data and encoding response data or
// errors.
type Codec interface {
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

// JSONCodec provides methods to encode data as JSON or decode data from JSON in
// HTTP requests and responses.
type JSONCodec struct{}

// NewJSONCodec creates a new JSONCodec instance.
func NewJSONCodec() JSONCodec {
	return JSONCodec{}
}

// Decode reads and decodes the JSON body of an HTTP request into the provided
// target struct or variable. Returns an error if decoding fails or if the
// request body is nil.
func (c JSONCodec) Decode(r *http.Request, into any) error {
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
func (c JSONCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	return writeJSON(w, data)
}

// EncodeError encodes an error into an HTTP response, handling
// `problem.DetailedError` if applicable to set the correct content type, or
// falling back to standard JSON encoding otherwise.
func (c JSONCodec) EncodeError(w http.ResponseWriter, err error) error {
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

// XMLCodec provides methods to encode data as XML or decode data from XML in
// HTTP requests and responses.
type XMLCodec struct{}

// NewXMLCodec creates a new XMLCodec instance.
func NewXMLCodec() XMLCodec {
	return XMLCodec{}
}

// Decode reads and decodes the XML body of an HTTP request into the provided
// target struct or variable. Returns an error if decoding fails or if the
// request body is nil.
func (c XMLCodec) Decode(r *http.Request, into any) error {
	if r.Body == nil {
		return nil
	}

	if err := xml.NewDecoder(r.Body).Decode(into); err != nil {
		return fmt.Errorf("decoding request body as XML: %w", err)
	}

	return nil
}

// Encode writes the given data as XML to the provided HTTP response writer
// with the appropriate Content-Type header.
func (c XMLCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	return writeXML(w, data)
}

// EncodeError encodes an error into an HTTP response, handling
// `problem.DetailedError` if applicable to set the correct content type, or
// falling back to standard XML encoding otherwise.
func (c XMLCodec) EncodeError(w http.ResponseWriter, err error) error {
	var problemDetails *problem.DetailedError
	if errors.As(err, &problemDetails) {
		w.Header().Set("Content-Type", "application/problem+xml; charset=utf-8")
		return writeXML(w, problemDetails)
	}

	return c.Encode(w, err)
}

// writeXML writes the given data as XML to the provided writer. It returns an
// error if encoding fails.
func writeXML(w io.Writer, data any) error {
	if err := xml.NewEncoder(w).Encode(data); err != nil {
		return fmt.Errorf("encoding response data as XML: %w", err)
	}

	return nil
}

// HTMXCodec provides methods to encode data as HTML or decode data from HTML in
// HTTP requests and responses.
//
// This codec is intended to be used with the [HTMX](https://htmx.org/) library.
// It uses the [form](https://github.com/go-playground/form) library to parse
// the form data and [template](https://pkg.go.dev/html/template) to render the
// HTML.
type HTMXCodec struct {
	decoder   *form.Decoder
	templates *template.Template
}

// NewHTMXCodec creates a new HTMXCodec instance.
func NewHTMXCodec(templates *template.Template) HTMXCodec {
	return HTMXCodec{
		decoder:   form.NewDecoder(),
		templates: templates,
	}
}

// Decode reads and decodes the form data of an HTTP request into the provided
// target struct or variable. Returns an error if decoding fails or if the
// request body is nil.
//
// This codec uses the [form](https://github.com/go-playground/form) library to
// parse the form data.
func (c HTMXCodec) Decode(r *http.Request, into any) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("parsing form: %w", err)
	}

	if err := c.decoder.Decode(into, r.Form); err != nil {
		return fmt.Errorf("decoding form: %w", err)
	}

	return nil
}

type templateData struct {
	name string
	data any
}

var errTemplateDataMissing = errors.New("template data missing")

func (c HTMXCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, ok := data.(templateData)
	if !ok {
		return errTemplateDataMissing
	}

	if err := c.templates.ExecuteTemplate(w, tmpl.name, tmpl.data); err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}

	return nil
}

func (c HTMXCodec) EncodeError(w http.ResponseWriter, _ error) error {
	return nil
}
