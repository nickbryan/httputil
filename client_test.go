package httputil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

var (
	httpMethods = []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}
	successCodes = []int{
		// 200 range.
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNonAuthoritativeInfo,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusPartialContent,
		http.StatusMultiStatus,
		http.StatusAlreadyReported,
		http.StatusIMUsed,
	}
	errorCodes = []int{
		// 400 range.
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusProxyAuthRequired,
		http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusTeapot,
		http.StatusMisdirectedRequest,
		http.StatusUnprocessableEntity,
		http.StatusLocked,
		http.StatusFailedDependency,
		http.StatusTooEarly,
		http.StatusUpgradeRequired,
		http.StatusPreconditionRequired,
		http.StatusTooManyRequests,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons,
		// 500 range.
		http.StatusInternalServerError,
		http.StatusNotImplemented,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusHTTPVersionNotSupported,
		http.StatusVariantAlsoNegotiates,
		http.StatusInsufficientStorage,
		http.StatusLoopDetected,
		http.StatusNotExtended,
		http.StatusNetworkAuthenticationRequired,
	}
)

func TestClient(t *testing.T) {
	t.Parallel()

	t.Run("handles a successful response from the server", func(t *testing.T) {
		t.Parallel()

		type testData struct {
			method string
			code   int
		}

		testCases := make([]testData, 0, len(successCodes)*len(httpMethods))

		for _, method := range httpMethods {
			for _, code := range successCodes {
				testCases = append(testCases, testData{method: method, code: code})
			}
		}

		for _, testCase := range testCases {
			t.Run(testCase.method+" "+strconv.Itoa(testCase.code), func(t *testing.T) {
				t.Parallel()

				client := newSuccessServerClient(t)

				res, err := callClientMethod(t, client, testCase.method, httputil.WithRequestParam("code", strconv.Itoa(testCase.code)))
				if err != nil {
					t.Fatalf("unexpected error from client call: %s", err.Error())
				}

				if res.StatusCode != testCase.code {
					t.Errorf("unexpected status code, want: %d, got: %d", testCase.code, res.StatusCode)
				}

				if !res.IsSuccess() {
					t.Errorf("unexpected IsSuccess() result, want: true, got: false")
				}

				if res.IsError() {
					t.Errorf("unexpected IsError() result, want: false, got: true")
				}

				if testCase.code == http.StatusNoContent {
					return
				}

				var got struct {
					Status string `json:"status"`
				}

				if err = res.Decode(&got); err != nil {
					t.Errorf("unexpected error decoding response for status %d: %s", testCase.code, err.Error())
				}

				if got.Status != strconv.Itoa(testCase.code) {
					t.Errorf("unexpected status in decoded response, want: %d, got: %s", testCase.code, got.Status)
				}
			})
		}
	})

	t.Run("handles an unsuccessful response from the server", func(t *testing.T) {
		t.Parallel()

		type testData struct {
			method string
			code   int
		}

		testCases := make([]testData, 0, len(errorCodes)*len(httpMethods))

		for _, method := range httpMethods {
			for _, code := range errorCodes {
				testCases = append(testCases, testData{method: method, code: code})
			}
		}

		for _, testCase := range testCases {
			t.Run(testCase.method+" "+strconv.Itoa(testCase.code), func(t *testing.T) {
				t.Parallel()

				client := newErrServerClient(t)

				res, err := callClientMethod(t, client, testCase.method, httputil.WithRequestParam("code", strconv.Itoa(testCase.code)))
				if err != nil {
					t.Fatalf("unexpected error from client call: %s", err.Error())
				}

				if res.StatusCode != testCase.code {
					t.Errorf("unexpected status code, want: %d, got: %d", testCase.code, res.StatusCode)
				}

				if res.IsSuccess() {
					t.Errorf("unexpected IsSuccess() result, want: false, got: true")
				}

				if !res.IsError() {
					t.Errorf("unexpected IsError() result, want: true, got: false")
				}

				if testCase.code == http.StatusNoContent {
					return
				}

				got, err := res.AsProblemDetails()
				if err != nil {
					t.Errorf("unexpected error getting problem details for status %d: %s", testCase.code, err.Error())
				}

				if got.Status != testCase.code {
					t.Errorf("unexpected status in problem details, want: %d, got: %d", testCase.code, got.Status)
				}
			})
		}
	})

	t.Run("returns an error when building the request fails", func(t *testing.T) {
		t.Parallel()

		// Setting ":" as the base path will cause the client to fail to build and parse
		// the request URL with "missing protocol scheme".
		client := httputil.NewClient(httputil.WithClientBasePath(":"))

		for _, method := range httpMethods {
			t.Run(method, func(t *testing.T) {
				t.Parallel()

				_, err := callClientMethod(t, client, method)
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				message := "building request url: "
				if !strings.Contains(err.Error(), message) {
					t.Fatalf("expected error message to contain %q, got: %q", message, err.Error())
				}
			})
		}
	})

	t.Run("sends a request with an io.Reader as the body to the server", func(t *testing.T) {
		t.Parallel()

		client := newBodyAwareServerClient(t)

		for _, method := range httpMethods {
			t.Run(method, func(t *testing.T) {
				t.Parallel()

				// GET and DELETE methods do not have a request body.
				if method == http.MethodGet || method == http.MethodDelete {
					return
				}

				res, err := callClientMethodWithBody(t, client, method, bytes.NewBufferString("hello world"))
				if err != nil {
					t.Fatalf("unexpected error from client call: %s", err.Error())
				}

				if res.StatusCode != http.StatusOK {
					t.Errorf("unexpected status code, want: %d, got: %d", http.StatusOK, res.StatusCode)
				}

				type response struct {
					Content string `json:"content"`
				}

				var got response

				err = res.Decode(&got)
				if err != nil {
					t.Fatalf("failed to decode response: %s", err.Error())
				}

				if got.Content != "hello world" {
					t.Errorf("unexpected content, want: %q, got: %q", "hello world", got.Content)
				}
			})
		}
	})

	t.Run("sends a request with an encoded body to the server", func(t *testing.T) {
		t.Parallel()

		client := newBodyAwareServerClient(t)

		for _, method := range httpMethods {
			t.Run(method, func(t *testing.T) {
				t.Parallel()

				// GET and DELETE methods do not have a request body.
				if method == http.MethodGet || method == http.MethodDelete {
					return
				}

				type request struct {
					Message string `json:"message"`
				}

				res, err := callClientMethodWithBody(t, client, method, request{Message: "hello world"})
				if err != nil {
					t.Fatalf("unexpected error from client call: %s", err.Error())
				}

				if res.StatusCode != http.StatusOK {
					t.Errorf("unexpected status code, want: %d, got: %d", http.StatusOK, res.StatusCode)
				}

				type response struct {
					Content string `json:"content"`
				}

				var got response

				err = res.Decode(&got)
				if err != nil {
					t.Fatalf("failed to decode response: %s", err.Error())
				}

				if got.Content != `{"message":"hello world"}` {
					t.Errorf("unexpected content, want: %q, got: %q", `{"message":"hello world"}`, got.Content)
				}
			})
		}
	})

	t.Run("returns an error when encoding fails", func(t *testing.T) {
		t.Parallel()

		client := newSuccessServerClient(t, httputil.WithClientCodec(fakeCodec{
			contentType: "application/json",
			encode: func(any) (io.Reader, error) {
				return nil, errors.New("failed to encode")
			},
			decode: func(io.Reader, any) error {
				return nil
			},
		}))

		for _, method := range httpMethods {
			t.Run(method, func(t *testing.T) {
				t.Parallel()

				// GET and DELETE methods do not have a request body.
				if method == http.MethodGet || method == http.MethodDelete {
					return
				}

				type request struct {
					Message string `json:"message"`
				}

				res, err := callClientMethodWithBody(t, client, method, request{Message: "hello world"})
				if res != nil {
					t.Error("expected nil response, got non-nil")
				}

				if err == nil {
					t.Fatal("expected error, got nil")
				}

				message := "encoding request body: failed to encode"
				if !strings.Contains(err.Error(), message) {
					t.Fatalf("expected error message to contain %q, got: %q", message, err.Error())
				}
			})
		}
	})

	t.Run("returns an error when executing the request fails", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		server.Close()

		client := httputil.NewClient(httputil.WithClientBasePath(server.URL))

		for _, method := range httpMethods {
			t.Run(method, func(t *testing.T) {
				t.Parallel()

				_, err := callClientMethod(t, client, method)
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				message := "executing request: "
				if !strings.Contains(err.Error(), message) {
					t.Fatalf("expected error message to contain %q, got: %q", message, err.Error())
				}
			})
		}
	})

	t.Run("returns an error when decoding fails", func(t *testing.T) {
		t.Parallel()

		client := newSuccessServerClient(t, httputil.WithClientCodec(fakeCodec{
			contentType: "application/json",
			encode: func(any) (io.Reader, error) {
				return nil, nil
			},
			decode: func(io.Reader, any) error {
				return errors.New("failed to decode")
			},
		}))

		res, err := callClientMethod(t, client, http.MethodGet, httputil.WithRequestParam("code", strconv.Itoa(http.StatusOK)))
		if err != nil {
			t.Fatalf("unexpected error from client call: %s", err.Error())
		}

		var got struct {
			Status string `json:"status"`
		}

		err = res.Decode(&got)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		message := "failed to decode"
		if !strings.Contains(err.Error(), message) {
			t.Fatalf("expected error message to contain %q, got: %q", message, err.Error())
		}
	})

	t.Run("returns an error when decoding problem details fails", func(t *testing.T) {
		t.Parallel()

		client := newErrServerClient(t, httputil.WithClientCodec(fakeCodec{
			contentType: "application/problem+json",
			encode: func(any) (io.Reader, error) {
				return nil, nil
			},
			decode: func(io.Reader, any) error {
				return errors.New("failed to decode")
			},
		}))

		res, err := callClientMethod(t, client, http.MethodGet, httputil.WithRequestParam("code", strconv.Itoa(http.StatusBadRequest)))
		if err != nil {
			t.Fatalf("unexpected error from client call: %s", err.Error())
		}

		_, err = res.AsProblemDetails()
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		message := "failed to decode"
		if !strings.Contains(err.Error(), message) {
			t.Fatalf("expected error message to contain %q, got: %q", message, err.Error())
		}
	})

	t.Run("WithRequestHeader", func(t *testing.T) {
		t.Parallel()

		headerKey := "X-Test-Header"
		headerValue := "test-value"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(headerKey) != headerValue {
				t.Errorf("expected header %s to be %s, got %s", headerKey, headerValue, r.Header.Get(headerKey))
			}

			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		client := httputil.NewClient(httputil.WithClientBasePath(server.URL))

		_, err := client.Get(t.Context(), "/", httputil.WithRequestHeader(headerKey, headerValue))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("WithRequestHeaders", func(t *testing.T) {
		t.Parallel()

		headers := map[string]string{
			"X-Test-Header-1": "value-1",
			"X-Test-Header-2": "value-2",
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range headers {
				if r.Header.Get(k) != v {
					t.Errorf("expected header %s to be %s, got %s", k, v, r.Header.Get(k))
				}
			}

			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		client := httputil.NewClient(httputil.WithClientBasePath(server.URL))

		_, err := client.Get(t.Context(), "/", httputil.WithRequestHeaders(headers))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("WithRequestParam", func(t *testing.T) {
		t.Parallel()

		paramKey := "param1"
		paramValue := "value1"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get(paramKey) != paramValue {
				t.Errorf("expected query parameter %s to be %s, got %s", paramKey, paramValue, r.URL.Query().Get(paramKey))
			}

			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		client := httputil.NewClient(httputil.WithClientBasePath(server.URL))

		_, err := client.Get(t.Context(), "/", httputil.WithRequestParam(paramKey, paramValue))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("WithRequestParams", func(t *testing.T) {
		t.Parallel()

		params := map[string]string{
			"param1": "value1",
			"param2": "value2",
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range params {
				if r.URL.Query().Get(k) != v {
					t.Errorf("expected query parameter %s to be %s, got %s", k, v, r.URL.Query().Get(k))
				}
			}

			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		client := httputil.NewClient(httputil.WithClientBasePath(server.URL))

		_, err := client.Get(t.Context(), "/", httputil.WithRequestParams(params))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

type fakeCodec struct {
	contentType string
	encode      func(any) (io.Reader, error)
	decode      func(io.Reader, any) error
}

func (f fakeCodec) ContentType() string {
	return f.contentType
}

func (f fakeCodec) Encode(data any) (io.Reader, error) {
	return f.encode(data)
}

func (f fakeCodec) Decode(r io.Reader, into any) error {
	return f.decode(r, into)
}

func callClientMethod(t *testing.T, client *httputil.Client, method string, opts ...httputil.RequestOption) (*httputil.Result, error) {
	t.Helper()

	switch method {
	case http.MethodGet:
		return client.Get(t.Context(), "/", opts...)
	case http.MethodPost:
		return client.Post(t.Context(), "/", nil, opts...)
	case http.MethodPut:
		return client.Put(t.Context(), "/", nil, opts...)
	case http.MethodPatch:
		return client.Patch(t.Context(), "/", nil, opts...)
	case http.MethodDelete:
		return client.Delete(t.Context(), "/", opts...)
	default:
		t.Fatalf("unexpected method %s calling client", method)
		return nil, nil // Unreachable.
	}
}

func callClientMethodWithBody(t *testing.T, client *httputil.Client, method string, body any, opts ...httputil.RequestOption) (*httputil.Result, error) {
	t.Helper()

	switch method {
	case http.MethodPost:
		return client.Post(t.Context(), "/", body, opts...)
	case http.MethodPut:
		return client.Put(t.Context(), "/", body, opts...)
	case http.MethodPatch:
		return client.Patch(t.Context(), "/", body, opts...)
	default:
		t.Fatalf("unexpected method %s calling client with body", method)
		return nil, nil // Unreachable.
	}
}

func newSuccessServerClient(t *testing.T, opts ...httputil.ClientOption) *httputil.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseCode, err := strconv.Atoi(r.URL.Query().Get("code"))
		if err != nil {
			t.Fatalf("unexpected error parsing response code: %s", err.Error())
		}

		w.WriteHeader(responseCode)

		if responseCode != http.StatusNoContent {
			_, err = fmt.Fprintf(w, `{"status":"%d"}`, responseCode)
			if err != nil {
				t.Fatalf("unexpected error writing response: %s", err.Error())
			}
		}
	}))

	t.Cleanup(server.Close)

	return httputil.NewClient(append([]httputil.ClientOption{httputil.WithClientBasePath(server.URL)}, opts...)...)
}

func newErrServerClient(t *testing.T, opts ...httputil.ClientOption) *httputil.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseCode, err := strconv.Atoi(r.URL.Query().Get("code"))
		if err != nil {
			t.Fatalf("unexpected error parsing response code: %s", err.Error())
		}

		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(responseCode)

		if responseCode != http.StatusNoContent {
			p := problem.DetailedError{Status: responseCode}

			_, err = w.Write(p.MustMarshalJSON())
			if err != nil {
				t.Fatalf("unexpected error writing response: %s", err.Error())
			}
		}
	}))

	t.Cleanup(server.Close)

	return httputil.NewClient(append([]httputil.ClientOption{httputil.WithClientBasePath(server.URL)}, opts...)...)
}

func newBodyAwareServerClient(t *testing.T) *httputil.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("unexpected error reading request body: %s", err.Error())
		}

		if len(content) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		response := &struct {
			Content string `json:"content"`
		}{
			Content: string(content),
		}

		jsonResponse, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("unexpected error marshaling response: %s", err.Error())
		}

		w.Header().Set("Content-Type", "application/json")

		_, err = w.Write(jsonResponse)
		if err != nil {
			t.Fatalf("unexpected error writing response: %s", err.Error())
		}
	}))

	t.Cleanup(server.Close)

	return httputil.NewClient(httputil.WithClientBasePath(server.URL))
}
