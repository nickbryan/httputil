package httputil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
	"github.com/nickbryan/slogutil"
	"github.com/nickbryan/slogutil/slogmem"
)

type testUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestGenericFunctions(t *testing.T) {
	t.Parallel()

	t.Run("decodes a successful JSON response", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"1","name":"Alice"}`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		result, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if result.Data.ID != "1" || result.Data.Name != "Alice" {
			t.Errorf("Data = %+v, want {ID:1 Name:Alice}", result.Data)
		}

		if result.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
		}
	})

	t.Run("handles various 2xx status codes including boundaries", func(t *testing.T) {
		t.Parallel()

		codes := []int{
			http.StatusOK,
			http.StatusCreated,
			http.StatusAccepted,
			http.StatusNoContent,
			299,
		}

		for _, code := range codes {
			t.Run(fmt.Sprintf("status %d", code), func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(code)
				}))
				t.Cleanup(server.Close)

				logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
				client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

				result, err := httputil.Get[struct{}](t.Context(), client, "/")
				if err != nil {
					t.Fatalf("Get() error = %v", err)
				}

				if result.StatusCode != code {
					t.Errorf("StatusCode = %d, want %d", result.StatusCode, code)
				}
			})
		}
	})

	t.Run("struct{} type parameter skips decoding and drains body", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		result, err := httputil.Delete[struct{}](t.Context(), client, "/users/1", nil)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		if result.StatusCode != http.StatusNoContent {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusNoContent)
		}
	})

	t.Run("struct{} skips decode even when body is present", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"1","name":"Alice"}`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		result, err := httputil.Get[struct{}](t.Context(), client, "/users/1")
		if err != nil {
			t.Fatalf("Get[struct{}]() error = %v", err)
		}

		if result.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
		}
	})

	t.Run("non-struct{} type on empty body returns decode error", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err == nil {
			t.Fatal("expected decode error, got nil")
		}

		if !strings.Contains(err.Error(), "decoding response body") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "decoding response body")
		}
	})

	t.Run("returns ProblemResponseError for problem detail responses", func(t *testing.T) {
		t.Parallel()

		dummyReq := httptest.NewRequest(http.MethodGet, "/users/1", nil)
		pd := problem.NotFound(dummyReq)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)

			if err := json.NewEncoder(w).Encode(pd); err != nil {
				t.Errorf("encoding problem: %v", err)
			}
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		problemErr, ok := errors.AsType[*httputil.ProblemResponseError](err)
		if !ok {
			t.Fatalf("expected *ProblemResponseError, got %T: %v", err, err)
		}

		if problemErr.Problem.Code != "404-01" {
			t.Errorf("Problem.Code = %q, want %q", problemErr.Problem.Code, "404-01")
		}
	})

	t.Run("returns UnexpectedResponseError for non-problem error responses", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "something went wrong")
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		unexpectedErr, ok := errors.AsType[*httputil.UnexpectedResponseError](err)
		if !ok {
			t.Fatalf("expected *UnexpectedResponseError, got %T: %v", err, err)
		}

		if unexpectedErr.Response.StatusCode != http.StatusInternalServerError {
			t.Errorf("StatusCode = %d, want %d", unexpectedErr.Response.StatusCode, http.StatusInternalServerError)
		}
	})

	t.Run("custom client-level ErrorDecoder is called for error responses", func(t *testing.T) {
		t.Parallel()

		customErr := errors.New("custom decoded error")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(func(_ *http.Response, _ httputil.ClientCodec) error {
				return customErr
			}),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if !errors.Is(err, customErr) {
			t.Errorf("error = %v, want %v", err, customErr)
		}
	})

	t.Run("request-level ErrorDecoder overrides client-level", func(t *testing.T) {
		t.Parallel()

		clientErr := errors.New("client error decoder")
		requestErr := errors.New("request error decoder")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(func(_ *http.Response, _ httputil.ClientCodec) error {
				return clientErr
			}),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1",
			httputil.WithRequestErrorDecoder(func(_ *http.Response, _ httputil.ClientCodec) error {
				return requestErr
			}),
		)
		if !errors.Is(err, requestErr) {
			t.Errorf("error = %v, want %v", err, requestErr)
		}
	})

	t.Run("ErrorDecoder returning nil falls back to UnexpectedResponseError", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(func(_ *http.Response, _ httputil.ClientCodec) error {
				return nil
			}),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if _, ok := errors.AsType[*httputil.UnexpectedResponseError](err); !ok {
			t.Fatalf("expected *UnexpectedResponseError, got %T: %v", err, err)
		}
	})

	t.Run("returns an error when the base path is invalid", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath("https://example.com/%zz"))

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "building request url") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "building request url")
		}
	})

	t.Run("returns an error when the path is invalid", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath("https://example.com"))

		_, err := httputil.Get[testUser](t.Context(), client, "/users/%zz")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "parsing request path") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "parsing request path")
		}
	})

	t.Run("returns an error when executing the request fails", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		server.Close()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "executing request") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "executing request")
		}
	})

	t.Run("all HTTP methods work correctly", func(t *testing.T) {
		t.Parallel()

		testCases := map[string]struct {
			call func(t *testing.T, client *httputil.Client) (*httputil.Result[testUser], error)
		}{
			"GET": {
				call: func(t *testing.T, client *httputil.Client) (*httputil.Result[testUser], error) {
					t.Helper()
					return httputil.Get[testUser](t.Context(), client, "/")
				},
			},
			"POST": {
				call: func(t *testing.T, client *httputil.Client) (*httputil.Result[testUser], error) {
					t.Helper()
					return httputil.Post[testUser](t.Context(), client, "/", testUser{ID: "1"})
				},
			},
			"PUT": {
				call: func(t *testing.T, client *httputil.Client) (*httputil.Result[testUser], error) {
					t.Helper()
					return httputil.Put[testUser](t.Context(), client, "/", testUser{ID: "1"})
				},
			},
			"PATCH": {
				call: func(t *testing.T, client *httputil.Client) (*httputil.Result[testUser], error) {
					t.Helper()
					return httputil.Patch[testUser](t.Context(), client, "/", testUser{ID: "1"})
				},
			},
			"DELETE": {
				call: func(t *testing.T, client *httputil.Client) (*httputil.Result[testUser], error) {
					t.Helper()
					return httputil.Delete[testUser](t.Context(), client, "/", nil)
				},
			},
		}

		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != name {
						t.Errorf("method = %q, want %q", r.Method, name)
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, `{"id":"1","name":"Alice"}`)
				}))
				t.Cleanup(server.Close)

				logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
				client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

				result, err := tc.call(t, client)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if result.Data.ID != "1" {
					t.Errorf("Data.ID = %q, want %q", result.Data.ID, "1")
				}
			})
		}
	})

	t.Run("Post sends encoded body to server", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var received testUser
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decoding request body: %v", err)
			}

			if received.Name != "Alice" {
				t.Errorf("received.Name = %q, want %q", received.Name, "Alice")
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"new-id","name":"Alice"}`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		result, err := httputil.Post[testUser](t.Context(), client, "/users", testUser{Name: "Alice"})
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		if result.Data.ID != "new-id" {
			t.Errorf("Data.ID = %q, want %q", result.Data.ID, "new-id")
		}
	})

	t.Run("Post with io.Reader body passes through without encoding", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading body: %v", err)
			}

			if string(got) != "raw data" {
				t.Errorf("body = %q, want %q", string(got), "raw data")
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"1","name":"Alice"}`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		result, err := httputil.Post[testUser](t.Context(), client, "/", bytes.NewBufferString("raw data"))
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		if result.Data.ID != "1" {
			t.Errorf("Data.ID = %q, want %q", result.Data.ID, "1")
		}
	})

	t.Run("WithRequestParam appends query parameters to the request", func(t *testing.T) {
		t.Parallel()

		var gotQuery string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[struct{}](t.Context(), client, "/",
			httputil.WithRequestParam("page", "1"),
			httputil.WithRequestParam("limit", "10"),
		)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		parsed, err := url.ParseQuery(gotQuery)
		if err != nil {
			t.Fatalf("parsing query: %v", err)
		}

		if got := parsed.Get("page"); got != "1" {
			t.Errorf("page = %q, want %q", got, "1")
		}

		if got := parsed.Get("limit"); got != "10" {
			t.Errorf("limit = %q, want %q", got, "10")
		}
	})

	t.Run("WithRequestParams adds multiple query parameters to the request", func(t *testing.T) {
		t.Parallel()

		var gotQuery string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[struct{}](t.Context(), client, "/",
			httputil.WithRequestParams(url.Values{"page": {"1"}, "limit": {"10"}}),
		)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		parsed, err := url.ParseQuery(gotQuery)
		if err != nil {
			t.Fatalf("parsing query: %v", err)
		}

		if got := parsed.Get("page"); got != "1" {
			t.Errorf("page = %q, want %q", got, "1")
		}

		if got := parsed.Get("limit"); got != "10" {
			t.Errorf("limit = %q, want %q", got, "10")
		}
	})

	t.Run("query params in path are merged with WithRequestParam", func(t *testing.T) {
		t.Parallel()

		var gotQuery string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[struct{}](t.Context(), client, "/?q=foo",
			httputil.WithRequestParam("page", "1"),
		)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		parsed, err := url.ParseQuery(gotQuery)
		if err != nil {
			t.Fatalf("parsing query: %v", err)
		}

		if got := parsed.Get("q"); got != "foo" {
			t.Errorf("q = %q, want %q", got, "foo")
		}

		if got := parsed.Get("page"); got != "1" {
			t.Errorf("page = %q, want %q", got, "1")
		}
	})

	t.Run("duplicate key in path and WithRequestParam preserves both values", func(t *testing.T) {
		t.Parallel()

		var gotQuery string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[struct{}](t.Context(), client, "/?tag=a",
			httputil.WithRequestParam("tag", "b"),
		)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		parsed, err := url.ParseQuery(gotQuery)
		if err != nil {
			t.Fatalf("parsing query: %v", err)
		}

		if got := parsed["tag"]; !strings.Contains(strings.Join(got, ","), "a") || !strings.Contains(strings.Join(got, ","), "b") {
			t.Errorf("tag = %v, want both %q and %q", got, "a", "b")
		}
	})

	t.Run("Delete with body sends encoded body to server", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var received testUser
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decoding request body: %v", err)
			}

			if received.ID != "42" {
				t.Errorf("received.ID = %q, want %q", received.ID, "42")
			}

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Delete[struct{}](t.Context(), client, "/users/42", testUser{ID: "42"})
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
	})

	t.Run("WithRequestHeader overrides default Accept header", func(t *testing.T) {
		t.Parallel()

		var gotAccept string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAccept = r.Header.Get("Accept")

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[struct{}](t.Context(), client, "/",
			httputil.WithRequestHeader("Accept", "text/plain"),
		)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if gotAccept != "text/plain" {
			t.Errorf("Accept = %q, want %q", gotAccept, "text/plain")
		}
	})

	t.Run("WithRequestHeader overrides default Content-Type header", func(t *testing.T) {
		t.Parallel()

		var gotContentType string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Post[struct{}](t.Context(), client, "/", testUser{Name: "Alice"},
			httputil.WithRequestHeader("Content-Type", "application/xml"),
		)
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		if gotContentType != "application/xml" {
			t.Errorf("Content-Type = %q, want %q", gotContentType, "application/xml")
		}
	})

	t.Run("io.Reader body still gets auto Content-Type header", func(t *testing.T) {
		t.Parallel()

		var gotContentType string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Post[struct{}](t.Context(), client, "/", strings.NewReader("raw data"))
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		want := "application/json; charset=utf-8"
		if gotContentType != want {
			t.Errorf("Content-Type = %q, want %q", gotContentType, want)
		}
	})

	t.Run("nil body omits Content-Type header", func(t *testing.T) {
		t.Parallel()

		var (
			gotContentType string
			hasContentType bool
		)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")
			_, hasContentType = r.Header["Content-Type"]

			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[struct{}](t.Context(), client, "/")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if hasContentType {
			t.Errorf("expected no Content-Type header, got %q", gotContentType)
		}
	})

	t.Run("returns encoding error when body encoding fails", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath("https://example.com"),
			httputil.WithClientCodec(&fakeClientCodec{
				contentType: "application/json",
				encode: func(_ any) (io.Reader, error) {
					return nil, errors.New("encode failed")
				},
			}),
		)

		_, err := httputil.Post[testUser](t.Context(), client, "/users", testUser{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "encoding request body") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "encoding request body")
		}
	})

	t.Run("logs body close errors", func(t *testing.T) {
		t.Parallel()

		closeErr := errors.New("close failed")

		logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath("https://example.com"),
			httputil.WithClientInterceptor(func(_ http.RoundTripper) http.RoundTripper {
				return httputil.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       &errorCloser{Reader: strings.NewReader(`{"id":"1","name":"Alice"}`), closeErr: closeErr},
					}, nil
				})
			}),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if ok, diff := logs.Contains(slogmem.RecordQuery{
			Level:   slog.LevelError,
			Message: "Failed to close response body",
		}); !ok {
			t.Errorf("expected close error log, diff: %s", diff)
		}
	})

	t.Run("logs body drain errors", func(t *testing.T) {
		t.Parallel()

		drainErr := errors.New("read failed")

		logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath("https://example.com"),
			httputil.WithClientInterceptor(func(_ http.RoundTripper) http.RoundTripper {
				return httputil.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body: io.NopCloser(io.MultiReader(
							strings.NewReader(`{"id":"1","name":"Alice"}`),
							iotest.ErrReader(drainErr),
						)),
					}, nil
				})
			}),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if ok, diff := logs.Contains(slogmem.RecordQuery{
			Level:   slog.LevelError,
			Message: "Failed to drain response body",
		}); !ok {
			t.Errorf("expected drain error log, diff: %s", diff)
		}
	})

	t.Run("returns decode error when problem response body is malformed", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `not valid json`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger, httputil.WithClientBasePath(server.URL))

		_, err := httputil.Get[testUser](t.Context(), client, "/users/1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "decoding problem response") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "decoding problem response")
		}
	})

	t.Run("ErrorDecoder receives the codec", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"message":"bad request"}`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(func(resp *http.Response, codec httputil.ClientCodec) error {
				var body struct {
					Message string `json:"message"`
				}

				if err := codec.Decode(resp.Body, &body); err != nil {
					return fmt.Errorf("decode: %w", err)
				}

				return fmt.Errorf("api error: %s", body.Message)
			}),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "api error: bad request") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "api error: bad request")
		}
	})

	t.Run("DecodeErrorAs decodes error response into typed error", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":"VALIDATION","message":"invalid input"}`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(httputil.DecodeErrorAs[*testAPIError]()),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		apiErr, ok := errors.AsType[*testAPIError](err)
		if !ok {
			t.Fatalf("expected *testAPIError, got %T: %v", err, err)
		}

		if apiErr.Code != "VALIDATION" {
			t.Errorf("Code = %q, want %q", apiErr.Code, "VALIDATION")
		}

		if apiErr.Message != "invalid input" {
			t.Errorf("Message = %q, want %q", apiErr.Message, "invalid input")
		}
	})

	t.Run("DecodeErrorAs returns decode error when body is malformed", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `not valid json`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(httputil.DecodeErrorAs[*testAPIError]()),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "decoding error response") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "decoding error response")
		}
	})

	t.Run("ErrorDecoder that closes body does not cause issues on deferred close", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":"CLOSED","message":"decoder closed body"}`)
		}))
		t.Cleanup(server.Close)

		logger, logs := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(func(resp *http.Response, codec httputil.ClientCodec) error {
				var apiErr testAPIError
				if err := codec.Decode(resp.Body, &apiErr); err != nil {
					return fmt.Errorf("decode: %w", err)
				}

				resp.Body.Close()

				return &apiErr
			}),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		apiErr, ok := errors.AsType[*testAPIError](err)
		if !ok {
			t.Fatalf("expected *testAPIError, got %T: %v", err, err)
		}

		if apiErr.Code != "CLOSED" {
			t.Errorf("Code = %q, want %q", apiErr.Code, "CLOSED")
		}

		if ok, _ := logs.Contains(slogmem.RecordQuery{
			Level:   slog.LevelError,
			Message: "Failed to drain response body",
		}); ok {
			t.Error("unexpected drain error log when body was closed by ErrorDecoder")
		}

		if ok, _ := logs.Contains(slogmem.RecordQuery{
			Level:   slog.LevelError,
			Message: "Failed to close response body",
		}); ok {
			t.Error("unexpected close error log when body was closed by ErrorDecoder")
		}
	})

	t.Run("DecodeErrorAs falls back to UnexpectedResponseError for null body", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `null`)
		}))
		t.Cleanup(server.Close)

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger,
			httputil.WithClientBasePath(server.URL),
			httputil.WithClientErrorDecoder(httputil.DecodeErrorAs[*testAPIError]()),
		)

		_, err := httputil.Get[testUser](t.Context(), client, "/")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if _, ok := errors.AsType[*httputil.UnexpectedResponseError](err); !ok {
			t.Fatalf("expected *UnexpectedResponseError, got %T: %v", err, err)
		}
	})
}

type testAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *testAPIError) Error() string {
	return e.Code + ": " + e.Message
}

type errorCloser struct {
	io.Reader

	closeErr error
}

func (e *errorCloser) Close() error {
	return e.closeErr
}
