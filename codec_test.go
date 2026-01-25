package httputil_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestJSONClientCodec_Decode(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Foo string `json:"foo"`
	}

	testCases := map[string]struct {
		reader    io.Reader
		into      any
		wantErr   bool
		wantErrAs error
		wantPanic bool
		wantVal   any
	}{
		"panics when reader is nil": {
			reader:    nil,
			into:      &testStruct{},
			wantPanic: true,
		},
		"returns an error when into is nil and reader is not empty": {
			reader:  strings.NewReader(`{"foo":"bar"}`),
			into:    nil,
			wantErr: true,
		},
		"returns an error for malformed JSON": {
			reader:  strings.NewReader("foo"),
			into:    &testStruct{},
			wantErr: true,
		},
		"decodes valid JSON": {
			reader:  strings.NewReader(`{"foo":"bar"}`),
			into:    &testStruct{},
			wantErr: false,
			wantVal: &testStruct{Foo: "bar"},
		},
		"returns io.EOF for empty reader": {
			reader:    strings.NewReader(""),
			into:      &testStruct{},
			wantErr:   true,
			wantErrAs: io.EOF,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			codec := httputil.NewJSONClientCodec()

			if tc.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Error("expected a panic")
					}
				}()
			}

			err := codec.Decode(tc.reader, tc.into)

			if (err != nil) != tc.wantErr {
				t.Fatalf("Decode() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.wantErrAs != nil && !errors.Is(err, tc.wantErrAs) {
				t.Fatalf("Decode() error = %v, wantErrAs %v", err, tc.wantErrAs)
			}

			if !tc.wantErr {
				if diff := cmp.Diff(tc.wantVal, tc.into); diff != "" {
					t.Errorf("Decode() into mismatch (-want +got):\n%s", diff)
				}
			}
		})
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

type jsonUnsupportedError struct {
	err any
}

func (e *jsonUnsupportedError) Error() string {
	return "json unsupported error"
}

func (e *jsonUnsupportedError) MarshalJSON() ([]byte, error) {
	return nil, &json.UnsupportedTypeError{}
}
