package httputil_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

func TestIsProblem(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		contentType string
		want        bool
	}{
		"returns true for application/problem+json": {
			contentType: "application/problem+json",
			want:        true,
		},
		"returns true for application/problem+json with charset": {
			contentType: "application/problem+json; charset=utf-8",
			want:        true,
		},
		"returns true for application/problem+xml": {
			contentType: "application/problem+xml",
			want:        true,
		},
		"returns true for application/problem+xml with charset": {
			contentType: "application/problem+xml; charset=utf-8",
			want:        true,
		},
		"returns false for application/json": {
			contentType: "application/json",
			want:        false,
		},
		"returns false for text/plain": {
			contentType: "text/plain",
			want:        false,
		},
		"returns false for empty Content-Type": {
			contentType: "",
			want:        false,
		},
		"returns false for malformed Content-Type": {
			contentType: ";;;",
			want:        false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{Header: http.Header{}}
			if tc.contentType != "" {
				resp.Header.Set("Content-Type", tc.contentType)
			}

			if got := httputil.IsProblem(resp); got != tc.want {
				t.Errorf("IsProblem() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProblemResponseError(t *testing.T) {
	t.Parallel()

	dummyReq := httptest.NewRequest(http.MethodGet, "/", nil)

	t.Run("implements error interface wrapping the problem detail", func(t *testing.T) {
		t.Parallel()

		pd := problem.NotFound(dummyReq)
		err := &httputil.ProblemResponseError{
			Response: &http.Response{StatusCode: http.StatusNotFound},
			Problem:  pd,
		}

		want := "problem response: " + pd.Error()
		if got := err.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("unwraps to the underlying DetailedError", func(t *testing.T) {
		t.Parallel()

		pd := problem.ResourceExists(dummyReq).WithDetail("duplicate entry")
		err := &httputil.ProblemResponseError{
			Response: &http.Response{StatusCode: http.StatusConflict},
			Problem:  pd,
		}

		wrapped := fmt.Errorf("calling API: %w", err)

		target, ok := errors.AsType[*problem.DetailedError](wrapped)
		if !ok {
			t.Fatal("errors.AsType did not match *problem.DetailedError")
		}

		if target.Status != http.StatusConflict {
			t.Errorf("Status = %d, want %d", target.Status, http.StatusConflict)
		}

		if target.Detail != "duplicate entry" {
			t.Errorf("Detail = %q, want %q", target.Detail, "duplicate entry")
		}
	})

	t.Run("can be matched with errors.AsType", func(t *testing.T) {
		t.Parallel()

		pd := problem.BadRequest(dummyReq)
		original := &httputil.ProblemResponseError{
			Response: &http.Response{StatusCode: http.StatusBadRequest},
			Problem:  pd,
		}

		wrapped := fmt.Errorf("calling API: %w", original)

		target, ok := errors.AsType[*httputil.ProblemResponseError](wrapped)
		if !ok {
			t.Fatal("errors.AsType did not match *ProblemResponseError")
		}

		if target.Problem.Status != http.StatusBadRequest {
			t.Errorf("Problem.Status = %d, want %d", target.Problem.Status, http.StatusBadRequest)
		}
	})
}

func TestUnexpectedResponseError(t *testing.T) {
	t.Parallel()

	t.Run("implements error interface with status code", func(t *testing.T) {
		t.Parallel()

		err := &httputil.UnexpectedResponseError{
			Response: &http.Response{StatusCode: http.StatusBadGateway},
		}

		want := "unexpected response status: 502"
		if got := err.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("can be matched with errors.AsType", func(t *testing.T) {
		t.Parallel()

		original := &httputil.UnexpectedResponseError{
			Response: &http.Response{StatusCode: http.StatusServiceUnavailable},
		}

		wrapped := fmt.Errorf("calling API: %w", original)

		target, ok := errors.AsType[*httputil.UnexpectedResponseError](wrapped)
		if !ok {
			t.Fatal("errors.AsType did not match *UnexpectedResponseError")
		}

		if target.Response.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Response.StatusCode = %d, want %d", target.Response.StatusCode, http.StatusServiceUnavailable)
		}
	})
}
