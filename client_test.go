package httputil_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/nickbryan/httputil"
)

func TestClient(t *testing.T) {
	t.Parallel()

	t.Run("handles a successful response form the server", func(t *testing.T) {
		t.Parallel()

		type response struct {
			Status string `json:"status"`
		}

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

		client := httputil.NewClient(httputil.WithClientBasePath(server.URL))

		for _, method := range []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		} {
			for _, code := range []int{
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
			} {
				t.Run(method+" "+strconv.Itoa(code), func(t *testing.T) {
					t.Parallel()

					var (
						res *httputil.Result
						err error
					)

					codeParam := httputil.WithRequestParam("code", strconv.Itoa(code))

					switch method {
					case http.MethodGet:
						res, err = client.Get(t.Context(), "/", codeParam)
					case http.MethodPost:
						res, err = client.Post(t.Context(), "/", nil, codeParam)
					case http.MethodPut:
						res, err = client.Put(t.Context(), "/", nil, codeParam)
					case http.MethodPatch:
						res, err = client.Patch(t.Context(), "/", nil, codeParam)
					case http.MethodDelete:
						res, err = client.Delete(t.Context(), "/", codeParam) // Delete requests cannot return a response body.
					default:
						t.Fatalf("unexpected method: %s", method)
					}

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

					if code == http.StatusNoContent {
						return
					}

					var got response
					err = res.Decode(&got)
					if err != nil {
						t.Errorf("unexpected error from res.Decode: %s", err.Error())
					}

					if got.Status != strconv.Itoa(code) {
						t.Errorf("unexpected status, want: %s, got: %s", strconv.Itoa(code), got.Status)
					}
				})
			}
		}
	})
}
