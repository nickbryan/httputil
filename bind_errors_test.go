package httputil_test

import (
	"errors"
	"testing"

	"github.com/nickbryan/httputil"
)

func TestBindErrors_HasAny(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		errors httputil.BindErrors
		want   bool
	}{
		"returns false when no errors are set": {
			errors: httputil.BindErrors{},
			want:   false,
		},
		"returns true when only Data error is set": {
			errors: httputil.BindErrors{Data: errors.New("data error")},
			want:   true,
		},
		"returns true when only Params error is set": {
			errors: httputil.BindErrors{Params: errors.New("params error")},
			want:   true,
		},
		"returns true when both Data and Params errors are set": {
			errors: httputil.BindErrors{Data: errors.New("data error"), Params: errors.New("params error")},
			want:   true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tc.errors.HasAny(); got != tc.want {
				t.Errorf("HasAny() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBindErrors_Get(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		errors httputil.BindErrors
		field  string
		want   string
	}{
		"returns empty string when no errors are set": {
			errors: httputil.BindErrors{},
			field:  "email",
			want:   "",
		},
		"returns empty string when field is not in errors": {
			errors: httputil.BindErrors{Data: errors.New("some error")},
			field:  "unknown",
			want:   "",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tc.errors.Get(tc.field); got != tc.want {
				t.Errorf("Get(%q) = %q, want %q", tc.field, got, tc.want)
			}
		})
	}
}

func TestBindErrors_All(t *testing.T) {
	t.Parallel()

	t.Run("returns empty map when no errors are set", func(t *testing.T) {
		t.Parallel()

		be := httputil.BindErrors{}
		result := be.All()

		if result == nil {
			t.Fatal("All() returned nil, want non-nil empty map")
		}

		if len(result) != 0 {
			t.Errorf("All() returned %d elements, want 0", len(result))
		}
	})
}
