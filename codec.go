package httputil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-playground/form/v4"

	"github.com/nickbryan/httputil/problem"
)

// ClientCodec is an interface for encoding and decoding HTTP request and
// response bodies for the client. It provides methods for encoding request data
// and decoding response data or errors.
type ClientCodec interface {
	// ContentType returns the Content-Type header value for the client codec.
	ContentType() string
	// Encode encodes the given data into a new io.Reader.
	Encode(data any) (io.Reader, error)
	// Decode reads and decodes the response body into the provided target struct
	Decode(r io.Reader, into any) error
}

// JSONClientCodec provides methods to encode data as JSON or decode data from JSON in
// HTTP requests and responses.
type JSONClientCodec struct{}

// Ensure JSONClientCodec implements ClientCodec.
var _ ClientCodec = JSONClientCodec{}

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
	Encode(w http.ResponseWriter, statusCode int, data any) error
	// EncodeError encodes the provided error into the HTTP response writer and
	// returns an error if encoding fails.
	EncodeError(w http.ResponseWriter, statusCode int, err error) error
}

// JSONServerCodec provides methods to encode data as JSON or decode data from JSON in
// HTTP requests and responses.
type JSONServerCodec struct{}

// Ensure JSONServerCodec implements ServerCodec.
var _ ServerCodec = JSONServerCodec{}

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
func (c JSONServerCodec) Encode(w http.ResponseWriter, statusCode int, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)

	return writeJSON(w, data)
}

// EncodeError encodes an error into an HTTP response, handling
// `problem.DetailedError` if applicable to set the correct content type, or
// falling back to standard JSON encoding otherwise.
func (c JSONServerCodec) EncodeError(w http.ResponseWriter, statusCode int, err error) error {
	if problemDetails, ok := errors.AsType[*problem.DetailedError](err); ok {
		w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
		w.WriteHeader(statusCode)

		return writeJSON(w, problemDetails)
	}

	return c.Encode(w, statusCode, err)
}

// writeJSON writes the given data as JSON to the provided writer. It returns an
// error if encoding fails.
func writeJSON(w io.Writer, data any) error {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return fmt.Errorf("encoding response data as JSON: %w", err)
	}

	return nil
}

// FormDecoder defines the interface for decoding form values into a struct.
// The default implementation uses [github.com/go-playground/form], but any
// compatible decoder can be substituted via [WithHTMLFormDecoder].
type FormDecoder interface {
	Decode(v any, values url.Values) error
}

// ErrTemplateNil is returned by [HTMLServerCodec.Encode] when the codec was
// constructed without a template set.
var ErrTemplateNil = errors.New("encoding response data as HTML: template is nil")

// EncodeTypeError is returned by [HTMLServerCodec.Encode] when the data
// argument is not an [httputil.Template].
type EncodeTypeError struct {
	Got any
}

// Error implements the error interface.
func (e *EncodeTypeError) Error() string {
	return fmt.Sprintf("encoding response data as HTML: data must be a httputil.Template, got %T", e.Got)
}

// HTMLServerCodecOption allows default HTMLServerCodec config values to be
// overridden.
type HTMLServerCodecOption func(c *HTMLServerCodec)

// WithHTMLErrorTemplate sets the name of a template within the main template
// set to use for rendering error pages. The named template receives a
// [*problem.DetailedError] as its data. If the named template is not found or
// the main template is nil, a minimal default error page is used.
func WithHTMLErrorTemplate(name string) HTMLServerCodecOption {
	return func(c *HTMLServerCodec) {
		c.errorTmplName = name
	}
}

// WithHTMLFormDecoder sets a custom [FormDecoder] for decoding form data. This
// allows full control over the form decoding strategy. If not set, a default
// decoder from [github.com/go-playground/form] is used.
func WithHTMLFormDecoder(decoder FormDecoder) HTMLServerCodecOption {
	return func(c *HTMLServerCodec) {
		c.decoder = decoder
	}
}

// WithHTMLMultipartMaxMemory sets the maximum number of bytes of a multipart
// form that will be stored in memory, with the remainder stored on disk. If
// not set, it defaults to 32 MB.
func WithHTMLMultipartMaxMemory(maxMemory int64) HTMLServerCodecOption {
	return func(c *HTMLServerCodec) {
		c.multipartMaxMemory = maxMemory
	}
}

// defaultErrorTemplate is the minimal HTML error page template used when no
// custom error template is provided or the named template is not found.
var defaultErrorTemplate = template.Must(template.New("error").Parse( //nolint:gochecknoglobals // Package-level default improves API.
	`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body><h1>{{.Title}}</h1><p>{{.Detail}}</p></body></html>`))

// HTMLServerCodec provides methods to decode form data from HTTP requests and
// encode response data as HTML using Go templates. Form data is decoded using
// an implementation of [FormDecoder], which defaults to
// [github.com/go-playground/form].
type HTMLServerCodec struct {
	decoder            FormDecoder
	errorTmpl          *template.Template
	errorTmplName      string
	multipartMaxMemory int64
	tmpl               *template.Template
}

// Ensure HTMLServerCodec implements ServerCodec.
var _ ServerCodec = HTMLServerCodec{} //nolint:exhaustruct // Compile time implementation check.

// NewHTMLServerCodec creates a new HTMLServerCodec instance configured with the
// provided template for rendering HTML responses. The template may be nil if
// only Decode functionality is required; however, Encode will return an error
// if called without a template. Options can be used to customize the error
// template name and form decoder.
func NewHTMLServerCodec(tmpl *template.Template, opts ...HTMLServerCodecOption) HTMLServerCodec {
	codec := HTMLServerCodec{
		decoder:            form.NewDecoder(),
		errorTmpl:          defaultErrorTemplate,
		errorTmplName:      "",
		multipartMaxMemory: defaultMaxMemory,
		tmpl:               tmpl,
	}

	for _, opt := range opts {
		opt(&codec)
	}

	if codec.errorTmplName != "" && codec.tmpl != nil {
		if t := codec.tmpl.Lookup(codec.errorTmplName); t != nil {
			codec.errorTmpl = t
		}
	}

	return codec
}

// defaultMaxMemory is the maximum number of bytes of a multipart form that
// will be stored in memory, with the remainder stored on disk. This matches
// the default used by [http.Request.FormValue].
const (
	defaultMaxMemory = 32 << 20 // 32 MB
)

// Decode parses the form data from an HTTP request and decodes it into the
// provided target struct. It supports both application/x-www-form-urlencoded
// and multipart/form-data content types for text fields. This method does not
// handle file uploads; use [http.Request.FormFile] or
// [http.Request.MultipartReader] directly for file access. Fields are mapped
// using the "form" struct tag. Returns nil when the request body is nil.
func (c HTMLServerCodec) Decode(r *http.Request, into any) error {
	if r.Body == nil {
		return nil
	}

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(c.multipartMaxMemory); err != nil {
			return fmt.Errorf("parsing multipart form data: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return fmt.Errorf("parsing form data: %w", err)
		}
	}

	if err := c.decoder.Decode(into, r.Form); err != nil {
		return fmt.Errorf("decoding form data: %w", err)
	}

	return nil
}

// Template wraps response data with a named template to execute from the
// template set. When passed as the data argument to [HTMLServerCodec.Encode],
// the named template is executed with the provided Data. For codecs that do not
// support template selection (such as [JSONServerCodec]), the Template struct is
// marshaled as-is.
//
// Usage:
//
//	return httputil.OK(httputil.Template{Name: "greeting", Data: myData})
type Template struct {
	// Name is the name of the template to execute from the template set.
	Name string
	// Data is passed to the template as its execution data.
	Data any
}

// Encode executes a named HTML template with the given data and writes the
// result to the HTTP response writer with the appropriate Content-Type header.
// The data argument must be a [Template] specifying the template name to
// execute from the template set and the data to pass to it. Returns an error
// if no template set is configured, if data is not a [Template], or if
// template execution fails.
func (c HTMLServerCodec) Encode(w http.ResponseWriter, statusCode int, data any) error {
	if c.tmpl == nil {
		return ErrTemplateNil
	}

	td, ok := data.(Template)
	if !ok {
		return &EncodeTypeError{Got: data}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)

	if err := c.tmpl.ExecuteTemplate(w, td.Name, td.Data); err != nil {
		return fmt.Errorf("encoding response data as HTML: %w", err)
	}

	return nil
}

// EncodeError encodes an error into an HTML HTTP response. The error template
// always receives a [*problem.DetailedError]. If the error is already a
// [*problem.DetailedError], it is used directly. Otherwise, a new
// [*problem.DetailedError] is constructed from the status code. The error
// template is resolved once during construction via [WithHTMLErrorTemplate].
func (c HTMLServerCodec) EncodeError(w http.ResponseWriter, statusCode int, err error) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)

	details, ok := errors.AsType[*problem.DetailedError](err)
	if !ok {
		details = &problem.DetailedError{
			Type:             "",
			Title:            http.StatusText(statusCode),
			Status:           statusCode,
			Detail:           "An unexpected error occurred.",
			Instance:         "",
			Code:             "",
			ExtensionMembers: nil,
		}
	}

	if execErr := c.errorTmpl.Execute(w, details); execErr != nil {
		return fmt.Errorf("encoding error response as HTML: %w", execErr)
	}

	return nil
}
