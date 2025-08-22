package httputil_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

func TestClient(t *testing.T) {
	t.Parallel()

	t.Run("handles a successful response form the server", func(t *testing.T) {
		t.Parallel()

		type response struct {
			Status string `json:"status"`
		}

		for _, method := range httpMethods(t) {
			for _, code := range successCodes(t) {
				t.Run(method+" "+strconv.Itoa(code), func(t *testing.T) {
					t.Parallel()

					client := newSuccessServerClient(t)

					res, err := callClientMethod(t, client, method, httputil.WithRequestParam("code", strconv.Itoa(code)))
					if err != nil {
						t.Errorf("unexpected error from client call: %s", err.Error())
						return
					}

					if res.StatusCode != code {
						t.Errorf("unexpected status code, want: %d, got: %d", code, res.StatusCode)
					}

					if !res.IsSuccess() {
						t.Errorf("unexpected IsSuccess() result, want: true, got: false")
					}

					if res.IsError() {
						t.Errorf("unexpected IsError() result, want: false, got: true")
					}

					if code == http.StatusNoContent {
						return
					}

					var got response
					err = res.Decode(&got)
					if err != nil {
						t.Errorf("unexpected error from res.Decode: %s", err.Error())
					}

					if got.Status != strconv.Itoa(code) {
						t.Errorf("unexpected status, want: %d, got: %s", code, got.Status)
					}
				})
			}
		}
	})

	t.Run("handles an unsuccessful response form the server", func(t *testing.T) {
		t.Parallel()

		for _, method := range httpMethods(t) {
			for _, code := range errorCodes(t) {
				t.Run(method+" "+strconv.Itoa(code), func(t *testing.T) {
					t.Parallel()

					client := newErrServerClient(t)

					res, err := callClientMethod(t, client, method, httputil.WithRequestParam("code", strconv.Itoa(code)))
					if err != nil {
						t.Errorf("unexpected error from client call: %s", err.Error())
						return
					}

					if res.StatusCode != code {
						t.Errorf("unexpected status code, want: %d, got: %d", code, res.StatusCode)
					}

					if !res.IsError() {
						t.Errorf("unexpected IsError() result, want: true, got: false")
					}

					if res.IsSuccess() {
						t.Errorf("unexpected IsSuccess() result, want: false, got: true")
					}

					if code == http.StatusNoContent {
						return
					}

					got, err := res.AsProblemDetails()
					if err != nil {
						t.Errorf("unexpected error from res.Decode: %s", err.Error())
					}

					if got.Status != code {
						t.Errorf("unexpected status, want: %d, got: %d", code, got.Status)
					}
				})
			}
		}
	})
}

func callClientMethod(t *testing.T, client *httputil.Client, method string, opts ...httputil.RequestOption) (*httputil.Result, error) {
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
		t.Fatalf("unexpected method: %s", method)
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
			_, err = w.Write([]byte(`{"status":"` + strconv.Itoa(responseCode) + `"}`))
			if err != nil {
				t.Fatalf("unexpected error writing response: %s", err.Error())
			}
		}
	}))

	return httputil.NewClient(httputil.WithClientBasePath(server.URL))

}

func newErrServerClient(t *testing.T) *httputil.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseCode, err := strconv.Atoi(r.URL.Query().Get("code"))
		if err != nil {
			t.Fatalf("unexpected error parsing response code: %s", err.Error())
		}

		w.WriteHeader(responseCode)

		if responseCode != http.StatusNoContent {
			p := problem.BadRequest(r)
			p.Status = responseCode

			_, err = w.Write(p.MustMarshalJSON())
			if err != nil {
				t.Fatalf("unexpected error writing response: %s", err.Error())
			}
		}
	}))

	return httputil.NewClient(httputil.WithClientBasePath(server.URL))
}

func httpMethods(t *testing.T) []string {
	t.Helper()

	return []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}
}

func successCodes(t *testing.T) []int {
	t.Helper()

	return []int{
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
}

func errorCodes(t *testing.T) []int {
	t.Helper()

	return []int{
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
}
