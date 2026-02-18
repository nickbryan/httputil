package httputil_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/nickbryan/httputil"
)

func TestRoundTripperFunc(t *testing.T) {
	t.Parallel()

	t.Run("passes arguments correctly through the underlying function", func(t *testing.T) {
		t.Parallel()

		resp, err := httputil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			code := r.URL.Query().Get("code")

			responseCode, err := strconv.Atoi(code)
			if err != nil {
				t.Fatalf("unexpected error parsing response code: %s", err.Error())
			}

			return &http.Response{StatusCode: responseCode}, errors.New("test error")
		}).RoundTrip(httptest.NewRequest(http.MethodGet, "/?code=418", nil)) //nolint:bodyclose // Nothing to close.
		if err == nil {
			t.Fatalf("expected error, got: nil")
		}

		if err.Error() != "test error" {
			t.Fatalf("unexpected error message, got: %q, want: %q", err.Error(), "test error")
		}

		if resp.StatusCode != http.StatusTeapot {
			t.Fatalf("unexpected status code, got: %d, want: %d", resp.StatusCode, http.StatusTeapot)
		}
	})
}
