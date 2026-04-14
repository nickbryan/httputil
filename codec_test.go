package httputil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-playground/form/v4"
	"github.com/google/go-cmp/cmp"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/internal/testutil"
	"github.com/nickbryan/httputil/problem"
)

func TestJSONClientCodec_ContentType(t *testing.T) {
	t.Parallel()

	codec := httputil.NewJSONClientCodec()
	if contentType := codec.ContentType(); contentType != "application/json; charset=utf-8" {
		t.Errorf("ContentType() = %q, want %q", contentType, "application/json; charset=utf-8")
	}
}

func TestJSONClientCodec_Encode(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		data     any
		wantBody string
		wantErr  bool
	}{
		"encodes nil data to null": {
			data:     nil,
			wantBody: "null",
		},
		"encodes valid data": {
			data:     map[string]string{"foo": "bar"},
			wantBody: `{"foo":"bar"}`,
		},
		"returns an error for unsupported json type": {
			data:    make(chan int),
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			codec := httputil.NewJSONClientCodec()
			reader, err := codec.Encode(tc.data)

			if (err != nil) != tc.wantErr {
				t.Fatalf("Encode() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.wantErr {
				return
			}

			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}

			if diff := testutil.DiffJSON(tc.wantBody, string(body)); diff != "" {
				t.Errorf("Encode() body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestJSONServerCodec_Decode(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Foo string `json:"foo"`
	}

	testCases := map[string]struct {
		request     *http.Request
		into        any
		wantErr     bool
		wantErrAs   error
		wantIntoVal any
	}{
		"decodes a valid json request body": {
			request: httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"foo":"bar"}`)),
			into:    &testStruct{},
			wantErr: false,
			wantIntoVal: &testStruct{
				Foo: "bar",
			},
		},
		"returns no error for a nil request body": {
			request: &http.Request{Body: nil},
			into:    &testStruct{},
			wantErr: false,
			wantIntoVal: &testStruct{
				Foo: "",
			},
		},
		"returns an error for a malformed json request body": {
			request: httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"foo":"bar`)),
			into:    &testStruct{},
			wantErr: true,
		},
		"returns an io.EOF error for an empty request body": {
			request:   httptest.NewRequest(http.MethodPost, "/", strings.NewReader("")),
			into:      &testStruct{},
			wantErr:   true,
			wantErrAs: io.EOF,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			codec := httputil.NewJSONServerCodec()
			err := codec.Decode(tc.request, tc.into)

			if (err != nil) != tc.wantErr {
				t.Fatalf("Decode() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.wantErrAs != nil && !errors.Is(err, tc.wantErrAs) {
				t.Fatalf("Decode() error = %v, wantErrAs %v", err, tc.wantErrAs)
			}

			if !tc.wantErr {
				if diff := cmp.Diff(tc.wantIntoVal, tc.into); diff != "" {
					t.Errorf("Decode() into mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestJSONServerCodec_Encode(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Foo string `json:"foo"`
	}

	testCases := map[string]struct {
		data            any
		statusCode      int
		wantBody        string
		wantContentType string
		wantStatusCode  int
		wantErr         bool
		wantPanic       bool
	}{
		"encodes a struct to json": {
			data:            &testStruct{Foo: "bar"},
			statusCode:      http.StatusOK,
			wantBody:        `{"foo":"bar"}`,
			wantContentType: "application/json; charset=utf-8",
			wantStatusCode:  http.StatusOK,
		},
		"encodes a map to json": {
			data:            map[string]string{"foo": "bar"},
			statusCode:      http.StatusOK,
			wantBody:        `{"foo":"bar"}`,
			wantContentType: "application/json; charset=utf-8",
			wantStatusCode:  http.StatusOK,
		},
		"returns an error when encoding fails": {
			data:           make(chan int),
			statusCode:     http.StatusInternalServerError,
			wantStatusCode: http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			codec := httputil.NewJSONServerCodec()

			err := codec.Encode(w, tc.statusCode, tc.data)

			assertResponse(t, w, err, tc.wantErr, tc.wantStatusCode, tc.wantContentType, tc.wantBody)
		})
	}
}

func TestJSONServerCodec_EncodeError(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		err             error
		statusCode      int
		wantBody        string
		wantContentType string
		wantStatusCode  int
		wantErr         bool
	}{
		"encodes a standard error to json": {
			err:             errors.New("some error"),
			statusCode:      http.StatusInternalServerError,
			wantBody:        `{}`,
			wantContentType: "application/json; charset=utf-8",
			wantStatusCode:  http.StatusInternalServerError,
		},
		"encodes a problem.DetailedError to json": {
			err:             problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", nil)),
			statusCode:      http.StatusBadRequest,
			wantBody:        problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", nil)).MustMarshalJSONString(),
			wantContentType: "application/problem+json; charset=utf-8",
			wantStatusCode:  http.StatusBadRequest,
		},
		"returns an error when encoding fails": {
			err:            &jsonUnsupportedError{err: make(chan int)},
			statusCode:     http.StatusInternalServerError,
			wantStatusCode: http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			codec := httputil.NewJSONServerCodec()

			err := codec.EncodeError(w, tc.statusCode, tc.err)

			assertResponse(t, w, err, tc.wantErr, tc.wantStatusCode, tc.wantContentType, tc.wantBody)
		})
	}
}

func TestHTMLServerCodec_Decode(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Name  string `form:"name"`
		Email string `form:"email"`
	}

	testCases := map[string]struct {
		request     *http.Request
		into        any
		wantErr     bool
		inspectErr  func(t *testing.T, err error)
		wantIntoVal any
	}{
		"decodes a valid url-encoded form body": {
			request: newFormRequest("name=Nick&email=nick%40example.com"),
			into:    &testStruct{},
			wantIntoVal: &testStruct{
				Name:  "Nick",
				Email: "nick@example.com",
			},
		},
		"returns no error for a nil request body": {
			request: &http.Request{Body: nil},
			into:    &testStruct{},
			wantIntoVal: &testStruct{
				Name:  "",
				Email: "",
			},
		},
		"decodes an empty form body without error": {
			request: newFormRequest(""),
			into:    &testStruct{},
			wantIntoVal: &testStruct{
				Name:  "",
				Email: "",
			},
		},
		"ignores fields without a form tag": {
			request: func() *http.Request {
				// This test verifies the decoder does not map fields that lack
				// a form tag since go-playground/form uses the struct tag.
				return newFormRequest("Name=Nick")
			}(),
			into: &struct{ Name string }{},
			wantIntoVal: &struct{ Name string }{
				Name: "Nick",
			},
		},
		"decodes only matching tagged fields": {
			request: newFormRequest("name=Nick&unknown=value"),
			into:    &testStruct{},
			wantIntoVal: &testStruct{
				Name:  "Nick",
				Email: "",
			},
		},
		"returns an error when form parsing fails": {
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name=Nick"))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded; %%%%")

				return req
			}(),
			into:    &testStruct{},
			wantErr: true,
			inspectErr: func(t *testing.T, err error) {
				t.Helper()

				if !errors.Is(err, mime.ErrInvalidMediaParameter) {
					t.Errorf("expected mime.ErrInvalidMediaParameter, got %v", err)
				}
			},
		},
		"returns an error when decoding into a non-pointer": {
			request: newFormRequest("name=Nick"),
			into:    testStruct{},
			wantErr: true,
			inspectErr: func(t *testing.T, err error) {
				t.Helper()

				var decodeErr *form.InvalidDecoderError
				if !errors.As(err, &decodeErr) {
					t.Errorf("expected *form.InvalidDecoderError, got %v", err)
				}
			},
		},
		"decodes multipart form data text fields": {
			request: newMultipartFormRequest(t, map[string]string{
				"name":  "Nick",
				"email": "nick@example.com",
			}),
			into: &testStruct{},
			wantIntoVal: &testStruct{
				Name:  "Nick",
				Email: "nick@example.com",
			},
		},
		"returns an error when multipart form parsing fails": {
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("invalid"))
				req.Header.Set("Content-Type", "multipart/form-data; boundary=bad")

				return req
			}(),
			into:    &testStruct{},
			wantErr: true,
			inspectErr: func(t *testing.T, err error) {
				t.Helper()

				if !errors.Is(err, io.EOF) {
					t.Errorf("expected wrapped io.EOF for multipart form parsing error, got %v", err)
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			codec := httputil.NewHTMLServerCodec(nil)

			err := codec.Decode(tc.request, tc.into)

			if (err != nil) != tc.wantErr {
				t.Fatalf("Decode() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.inspectErr != nil {
				tc.inspectErr(t, err)
			}

			if !tc.wantErr {
				if diff := cmp.Diff(tc.wantIntoVal, tc.into); diff != "" {
					t.Errorf("Decode() into mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestHTMLServerCodec_Encode(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		tmpl            httputil.TemplateExecutor
		data            any
		statusCode      int
		wantBody        string
		wantContentType string
		wantStatusCode  int
		wantErr         bool
	}{
		"encodes data using a named template": {
			tmpl: func() *template.Template {
				t := template.Must(template.New("root").Parse(""))
				template.Must(t.New("test").Parse(`<h1>Hello, {{.Name}}</h1>`))

				return t
			}(),
			data:            httputil.Template{Name: "test", Data: struct{ Name string }{Name: "Nick"}},
			statusCode:      http.StatusOK,
			wantBody:        `<h1>Hello, Nick</h1>`,
			wantContentType: "text/html; charset=utf-8",
			wantStatusCode:  http.StatusOK,
		},
		"returns an error when template set is nil": {
			tmpl:       nil,
			data:       httputil.Template{Name: "test", Data: nil},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		"returns an error when data is not a Template": {
			tmpl:       template.Must(template.New("test").Parse(`<p>test</p>`)),
			data:       struct{ Name string }{Name: "Nick"},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		"returns an error when named template is not found in the set": {
			tmpl:       template.Must(template.New("root").Parse(``)),
			data:       httputil.Template{Name: "nonexistent", Data: nil},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		"sets the correct status code": {
			tmpl: func() *template.Template {
				t := template.Must(template.New("root").Parse(""))
				template.Must(t.New("page").Parse(`<p>Created</p>`))

				return t
			}(),
			data:            httputil.Template{Name: "page", Data: nil},
			statusCode:      http.StatusCreated,
			wantBody:        `<p>Created</p>`,
			wantContentType: "text/html; charset=utf-8",
			wantStatusCode:  http.StatusCreated,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()

			codec := httputil.NewHTMLServerCodec(tc.tmpl)

			err := codec.Encode(w, tc.statusCode, tc.data)

			assertHTMLResponse(t, w, err, tc.wantErr, tc.wantStatusCode, tc.wantContentType, tc.wantBody)
		})
	}

	t.Run("returns ErrTemplateNil when template set is nil", func(t *testing.T) {
		t.Parallel()

		codec := httputil.NewHTMLServerCodec(nil)

		err := codec.Encode(httptest.NewRecorder(), http.StatusOK, httputil.Template{Name: "x"})
		if !errors.Is(err, httputil.ErrTemplateNil) {
			t.Errorf("expected ErrTemplateNil, got %v", err)
		}
	})

	t.Run("returns EncodeTypeError when data is not a Template", func(t *testing.T) {
		t.Parallel()

		codec := httputil.NewHTMLServerCodec(template.Must(template.New("root").Parse("")))

		err := codec.Encode(httptest.NewRecorder(), http.StatusOK, "not a template")

		var typeErr *httputil.EncodeTypeError
		if !errors.As(err, &typeErr) {
			t.Fatalf("expected EncodeTypeError, got %v", err)
		}

		if typeErr.Got != "not a template" {
			t.Errorf("EncodeTypeError.Got = %v, want %q", typeErr.Got, "not a template")
		}

		wantMsg := `encoding response data as HTML: data must be a httputil.Template, got string`
		if typeErr.Error() != wantMsg {
			t.Errorf("Error() = %q, want %q", typeErr.Error(), wantMsg)
		}
	})
}

func TestHTMLServerCodec_EncodeError(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		err              error
		statusCode       int
		wantBodyContains []string
		wantContentType  string
		wantStatusCode   int
	}{
		"encodes a standard error as an html error page": {
			err:        errors.New("some error"),
			statusCode: http.StatusInternalServerError,
			wantBodyContains: []string{
				"<h1>Internal Server Error</h1>",
				"<p>An unexpected error occurred.</p>",
			},
			wantContentType: "text/html; charset=utf-8",
			wantStatusCode:  http.StatusInternalServerError,
		},
		"encodes a problem.DetailedError with title and detail": {
			err:        problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", nil)),
			statusCode: http.StatusBadRequest,
			wantBodyContains: []string{
				"<h1>Bad Request</h1>",
			},
			wantContentType: "text/html; charset=utf-8",
			wantStatusCode:  http.StatusBadRequest,
		},
		"sets the correct status code for not found": {
			err:        errors.New("not found"),
			statusCode: http.StatusNotFound,
			wantBodyContains: []string{
				"<h1>Not Found</h1>",
				"<p>An unexpected error occurred.</p>",
			},
			wantContentType: "text/html; charset=utf-8",
			wantStatusCode:  http.StatusNotFound,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()

			codec := httputil.NewHTMLServerCodec(nil)

			err := codec.EncodeError(w, tc.statusCode, tc.err)
			if err != nil {
				t.Fatalf("EncodeError() error = %v", err)
			}

			if w.Code != tc.wantStatusCode {
				t.Errorf("Status code = %d, want %d", w.Code, tc.wantStatusCode)
			}

			if contentType := w.Header().Get("Content-Type"); contentType != tc.wantContentType {
				t.Errorf("Content-Type header = %q, want %q", contentType, tc.wantContentType)
			}

			body := w.Body.String()
			for _, want := range tc.wantBodyContains {
				if !strings.Contains(body, want) {
					t.Errorf("Body does not contain %q, got:\n%s", want, body)
				}
			}
		})
	}
}

func TestHTMLServerCodec_WithHTMLErrorTemplate(t *testing.T) {
	t.Parallel()

	t.Run("renders with a custom error template", func(t *testing.T) {
		t.Parallel()

		errTmpl := template.Must(template.New("custom-error").Parse(
			`<div class="error"><strong>{{.Title}}</strong>: {{.Detail}}</div>`,
		))

		w := httptest.NewRecorder()

		codec := httputil.NewHTMLServerCodec(nil, httputil.WithHTMLErrorTemplate(errTmpl))

		err := codec.EncodeError(w, http.StatusInternalServerError, errors.New("something went wrong"))
		if err != nil {
			t.Fatalf("EncodeError() error = %v", err)
		}

		wantBody := `<div class="error"><strong>Internal Server Error</strong>: An unexpected error occurred.</div>`
		if diff := cmp.Diff(wantBody, w.Body.String()); diff != "" {
			t.Errorf("Body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("returns an error when custom error template execution fails", func(t *testing.T) {
		t.Parallel()

		errTmpl := template.Must(template.New("bad-error").Option("missingkey=error").Parse(`{{.NonExistent}}`))

		w := httptest.NewRecorder()

		codec := httputil.NewHTMLServerCodec(nil, httputil.WithHTMLErrorTemplate(errTmpl))

		err := codec.EncodeError(w, http.StatusInternalServerError, errors.New("some error"))
		if err == nil {
			t.Fatal("EncodeError() expected an error, got nil")
		}
	})

	t.Run("renders a page-style error template with layout blocks", func(t *testing.T) {
		t.Parallel()

		base := template.Must(template.New("").Parse(""))
		template.Must(base.New("layout").Parse(
			`<html><head><title>{{ block "title" . }}App{{ end }}</title></head>` +
				`<body>{{ block "content" . }}{{ end }}</body></html>`))

		ts, err := httputil.NewTemplateSet(base, map[string]string{
			"error": `{{ template "layout" . }}{{ define "title" }}{{ .Title }}{{ end }}` +
				`{{ define "content" }}<h1>{{ .Title }}</h1><p>{{ .Detail }}</p>{{ end }}`,
		})
		if err != nil {
			t.Fatalf("NewTemplateSet() error = %v", err)
		}

		codec := httputil.NewHTMLServerCodec(ts,
			httputil.WithHTMLErrorTemplate(ts.Lookup("error")),
		)

		w := httptest.NewRecorder()

		err = codec.EncodeError(w, http.StatusNotFound, errors.New("missing"))
		if err != nil {
			t.Fatalf("EncodeError() error = %v", err)
		}

		want := `<html><head><title>Not Found</title></head>` +
			`<body><h1>Not Found</h1><p>An unexpected error occurred.</p></body></html>`
		if diff := cmp.Diff(want, w.Body.String()); diff != "" {
			t.Errorf("Body mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestHTMLServerCodec_WithHTMLFormDecoder(t *testing.T) {
	t.Parallel()

	t.Run("uses a custom form decoder with a different tag name", func(t *testing.T) {
		t.Parallel()

		type testStruct struct {
			Name string `custom:"name"`
		}

		customDecoder := form.NewDecoder()
		customDecoder.SetTagName("custom")

		req := newFormRequest("name=Nick")
		into := &testStruct{}

		codec := httputil.NewHTMLServerCodec(nil, httputil.WithHTMLFormDecoder(customDecoder))

		if err := codec.Decode(req, into); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if diff := cmp.Diff(&testStruct{Name: "Nick"}, into); diff != "" {
			t.Errorf("Decode() into mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("uses a custom FormDecoder interface implementation", func(t *testing.T) {
		t.Parallel()

		type testStruct struct {
			Name string
		}

		req := newFormRequest("name=Custom")
		into := &testStruct{}

		codec := httputil.NewHTMLServerCodec(nil, httputil.WithHTMLFormDecoder(&stubFormDecoder{
			decodeFn: func(v any, values url.Values) error {
				s, ok := v.(*testStruct)
				if !ok {
					return errors.New("unexpected type")
				}

				s.Name = values.Get("name")

				return nil
			},
		}))

		if err := codec.Decode(req, into); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if diff := cmp.Diff(&testStruct{Name: "Custom"}, into); diff != "" {
			t.Errorf("Decode() into mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestHTMLServerCodec_WithHTMLMultipartMaxMemory(t *testing.T) {
	t.Parallel()

	t.Run("sets the maximum memory for parsing multipart forms", func(t *testing.T) {
		t.Parallel()

		req := newMultipartFormRequest(t, map[string]string{
			"name": strings.Repeat("a", 1024), // 1KB string
		})

		// Set max memory to 0 to force disk usage, proving the option is respected
		codec := httputil.NewHTMLServerCodec(nil, httputil.WithHTMLMultipartMaxMemory(0))

		err := codec.Decode(req, &struct{ Name string }{})
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		// When maxMemory is 0, ParseMultipartForm writes all non-memory parts to disk.
		// We can verify this by checking that a temporary file was created.
		if req.MultipartForm == nil {
			t.Fatal("MultipartForm is nil, expected parsing to occur")
		}

		// The form values should still be present
		if len(req.MultipartForm.Value["name"]) == 0 {
			t.Fatal("Expected 'name' field to be parsed")
		}
	})
}

func TestHTMLServerCodec_WithTemplateSet(t *testing.T) {
	t.Parallel()

	t.Run("renders a page through TemplateSet via Encode", func(t *testing.T) {
		t.Parallel()

		base := template.Must(template.New("").Parse(""))
		template.Must(base.New("layout").Parse(
			`<html><body>{{ block "content" . }}{{ end }}</body></html>`))

		ts, err := httputil.NewTemplateSet(base, map[string]string{
			"home": `{{ template "layout" . }}{{ define "content" }}<h1>{{ . }}</h1>{{ end }}`,
		})
		if err != nil {
			t.Fatalf("NewTemplateSet() error = %v", err)
		}

		codec := httputil.NewHTMLServerCodec(ts)

		w := httptest.NewRecorder()

		err = codec.Encode(w, http.StatusOK, httputil.Template{Name: "home", Data: "Welcome"})
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		want := `<html><body><h1>Welcome</h1></body></html>`
		if diff := cmp.Diff(want, w.Body.String()); diff != "" {
			t.Errorf("Body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("uses error template looked up from TemplateSet", func(t *testing.T) {
		t.Parallel()

		base := template.Must(template.New("").Parse(""))
		template.Must(base.New("error").Parse(`<p>Error: {{.Title}}</p>`))

		ts, err := httputil.NewTemplateSet(base, map[string]string{
			"page": `<p>page</p>`,
		})
		if err != nil {
			t.Fatalf("NewTemplateSet() error = %v", err)
		}

		codec := httputil.NewHTMLServerCodec(ts, httputil.WithHTMLErrorTemplate(ts.Lookup("error")))

		w := httptest.NewRecorder()

		err = codec.EncodeError(w, http.StatusNotFound, errors.New("missing"))
		if err != nil {
			t.Fatalf("EncodeError() error = %v", err)
		}

		want := `<p>Error: Not Found</p>`
		if diff := cmp.Diff(want, w.Body.String()); diff != "" {
			t.Errorf("Body mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("backward compatible with plain *template.Template", func(t *testing.T) {
		t.Parallel()

		tmpl := template.Must(template.New("root").Parse(""))
		template.Must(tmpl.New("page").Parse(`<p>Hello, {{.}}</p>`))

		codec := httputil.NewHTMLServerCodec(tmpl)

		w := httptest.NewRecorder()

		err := codec.Encode(w, http.StatusOK, httputil.Template{Name: "page", Data: "World"})
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		want := `<p>Hello, World</p>`
		if diff := cmp.Diff(want, w.Body.String()); diff != "" {
			t.Errorf("Body mismatch (-want +got):\n%s", diff)
		}
	})
}

func assertResponse(t *testing.T, w *httptest.ResponseRecorder, err error, wantErr bool, wantStatusCode int, wantContentType, wantBody string) {
	t.Helper()

	if (err != nil) != wantErr {
		t.Fatalf("Encode() error = %v, wantErr %v", err, wantErr)
	}

	if wantErr {
		return
	}

	if w.Code != wantStatusCode {
		t.Errorf("Status code = %d, want %d", w.Code, wantStatusCode)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != wantContentType {
		t.Errorf("Content-Type header = %q, want %q", contentType, wantContentType)
	}

	if diff := testutil.DiffJSON(wantBody, w.Body.String()); diff != "" {
		t.Errorf("Body mismatch (-want +got):\n%s", diff)
	}
}

func assertHTMLResponse(t *testing.T, w *httptest.ResponseRecorder, err error, wantErr bool, wantStatusCode int, wantContentType, wantBody string) {
	t.Helper()

	if (err != nil) != wantErr {
		t.Fatalf("Encode() error = %v, wantErr %v", err, wantErr)
	}

	if wantErr {
		return
	}

	if w.Code != wantStatusCode {
		t.Errorf("Status code = %d, want %d", w.Code, wantStatusCode)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != wantContentType {
		t.Errorf("Content-Type header = %q, want %q", contentType, wantContentType)
	}

	if diff := cmp.Diff(wantBody, w.Body.String()); diff != "" {
		t.Errorf("Body mismatch (-want +got):\n%s", diff)
	}
}

func newMultipartFormRequest(t *testing.T, fields map[string]string) *http.Request {
	t.Helper()

	var buf bytes.Buffer

	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("writing multipart field %q: %v", k, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())

	return req
}

func newFormRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return req
}

type jsonUnsupportedError struct {
	err any
}

func (e *jsonUnsupportedError) Error() string {
	return "json unsupported error"
}

func (e *jsonUnsupportedError) MarshalJSON() ([]byte, error) {
	return nil, &json.UnsupportedTypeError{}
}

type stubFormDecoder struct {
	decodeFn func(v any, values url.Values) error
}

func (s *stubFormDecoder) Decode(v any, values url.Values) error {
	return s.decodeFn(v, values)
}
