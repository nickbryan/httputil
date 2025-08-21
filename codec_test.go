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

func TestJSONCodec_Decode(t *testing.T) {
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

func TestJSONCodec_Encode(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Foo string `json:"foo"`
	}

	testCases := map[string]struct {
		data               any
		wantBody           string
		wantContentType    string
		wantStatusCode     int
		wantErr            bool
		wantPanic          bool
		wantPanicSubstring string
	}{
		"encodes a struct to json": {
			data:            &testStruct{Foo: "bar"},
			wantBody:        `{"foo":"bar"}`,
			wantContentType: "application/json; charset=utf-8",
			wantStatusCode:  http.StatusOK,
		},
		"encodes a map to json": {
			data:            map[string]string{"foo": "bar"},
			wantBody:        `{"foo":"bar"}`,
			wantContentType: "application/json; charset=utf-8",
			wantStatusCode:  http.StatusOK,
		},
		"returns an error when encoding fails": {
			data:    make(chan int),
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			codec := httputil.NewJSONServerCodec()

			err := codec.Encode(w, tc.data)

			if (err != nil) != tc.wantErr {
				t.Fatalf("Encode() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.wantErr {
				return
			}

			if contentType := w.Header().Get("Content-Type"); contentType != tc.wantContentType {
				t.Errorf("Content-Type header = %q, want %q", contentType, tc.wantContentType)
			}

			if diff := testutil.DiffJSON(tc.wantBody, w.Body.String()); diff != "" {
				t.Errorf("Body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestJSONCodec_EncodeError(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		err             error
		wantBody        string
		wantContentType string
		wantErr         bool
	}{
		"encodes a standard error to json": {
			err:             errors.New("some error"),
			wantBody:        `{}`,
			wantContentType: "application/json; charset=utf-8",
		},
		"encodes a problem.DetailedError to json": {
			err:             problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", nil)),
			wantBody:        problem.BadRequest(httptest.NewRequest(http.MethodGet, "/test", nil)).MustMarshalJSONString(),
			wantContentType: "application/problem+json; charset=utf-8",
		},
		"returns an error when encoding fails": {
			err:     &jsonUnsupportedError{err: make(chan int)},
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			codec := httputil.NewJSONServerCodec()

			err := codec.EncodeError(w, tc.err)

			if (err != nil) != tc.wantErr {
				t.Fatalf("EncodeError() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.wantErr {
				return
			}

			if contentType := w.Header().Get("Content-Type"); contentType != tc.wantContentType {
				t.Errorf("Content-Type header = %q, want %q", contentType, tc.wantContentType)
			}

			if diff := testutil.DiffJSON(tc.wantBody, w.Body.String()); diff != "" {
				t.Errorf("Body mismatch (-want +got):\n%s", diff)
			}
		})
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
