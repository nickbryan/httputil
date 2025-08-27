package httputil_test

import (
	"bytes"
	"encoding/json"
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

		var testCases []testData
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
					t.Errorf("unexpected error from res.Decode: %s", err.Error())
				}
				if got.Status != strconv.Itoa(testCase.code) {
					t.Errorf("unexpected status, want: %d, got: %s", testCase.code, got.Status)
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

		var testCases []testData
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
					t.Errorf("unexpected error from res.Decode: %s", err.Error())
				}
				if got.Status != testCase.code {
					t.Errorf("unexpected status, want: %d, got: %d", testCase.code, got.Status)
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
					t.Fatalf("unexpected error message, got: %q, want: strings.Contains(%q)", err.Error(), message)
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
					t.Fatalf("unexpected error from res.Decode: %s", err.Error())
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
					t.Fatalf("unexpected error from res.Decode: %s", err.Error())
				}

				if got.Content != `{"message":"hello world"}` {
					t.Errorf("unexpected content, want: %q, got: %q", `{"message":"hello world"}`, got.Content)
				}
			})
		}
	})
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

func newSuccessServerClient(t *testing.T) *httputil.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseCode, err := strconv.Atoi(r.URL.Query().Get("code"))
		if err != nil {
			t.Fatalf("unexpected error parsing response code: %s", err.Error())
		}

		w.WriteHeader(responseCode)

		if responseCode != http.StatusNoContent {
			_, err = w.Write([]byte(fmt.Sprintf(`{"status":"%d"}`, responseCode)))
			if err != nil {
				t.Fatalf("unexpected error writing response: %s", err.Error())
			}
		}
	}))

	t.Cleanup(server.Close)

	return httputil.NewClient(httputil.WithClientBasePath(server.URL))
}

func newErrServerClient(t *testing.T) *httputil.Client {
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

	return httputil.NewClient(httputil.WithClientBasePath(server.URL))
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
			t.Fatalf("unexpected error marshalling response: %s", err.Error())
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
