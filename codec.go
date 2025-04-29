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

type Codec interface {
	// Decode decodes the request data and sets it on into.
	// Implementations of Decode should return [io.EOF] if
	// the request data is empty when Decode is called.
	Decode(r *http.Request, into any) error
	Encode(w http.ResponseWriter, data any) error
	EncodeError(w http.ResponseWriter, err error) error
}

type JSONCodec struct{}

func NewJSONCodec() *JSONCodec {
	return &JSONCodec{}
}

func (c JSONCodec) Decode(r *http.Request, into any) error {
	if r.Body == nil {
		return nil
	}

	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		return fmt.Errorf("decoding request body as JSON: %w", err)
	}

	return nil
}

func (c JSONCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	return writeJSON(w, data)
}

func (c JSONCodec) EncodeError(w http.ResponseWriter, err error) error {
	var problemDetails *problem.DetailedError
	if errors.As(err, &problemDetails) {
		w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
		return writeJSON(w, problemDetails)
	}

	return c.Encode(w, err)
}

func writeJSON(w io.Writer, data any) error {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return fmt.Errorf("encoding response data as JSON: %w", err)
	}

	return nil
}

type XMLCodec struct{}

func NewXMLCodec() *XMLCodec {
	return &XMLCodec{}
}

func (c XMLCodec) Decode(r *http.Request, into any) error {
	if r.Body == nil {
		return nil
	}

	if err := xml.NewDecoder(r.Body).Decode(into); err != nil {
		return fmt.Errorf("decoding request body as XML: %w", err)
	}

	return nil
}

func (c XMLCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	return writeXML(w, data)
}

func (c XMLCodec) EncodeError(w http.ResponseWriter, err error) error {
	var problemDetails *problem.DetailedError
	if errors.As(err, &problemDetails) {
		w.Header().Set("Content-Type", "application/problem+xml; charset=utf-8")
		return writeXML(w, problemDetails)
	}

	return c.Encode(w, err)
}

func writeXML(w io.Writer, data any) error {
	if err := xml.NewEncoder(w).Encode(data); err != nil {
		return fmt.Errorf("encoding response data as XML: %w", err)
	}

	return nil
}

type HTMXCodec struct {
	decoder   *form.Decoder
	templates *template.Template
}

func NewHTMXCodec(templates *template.Template) *HTMXCodec {
	return &HTMXCodec{
		decoder:   form.NewDecoder(),
		templates: templates,
	}
}

func (c HTMXCodec) Decode(r *http.Request, into any) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("parsing form: %w", err)
	}

	if err := c.decoder.Decode(into, r.Form); err != nil {
		return fmt.Errorf("decoding form: %w", err)
	}

	return nil
}

func (c HTMXCodec) Encode(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	return nil
}

func (c HTMXCodec) EncodeError(w http.ResponseWriter, err error) error {
	return nil
}
