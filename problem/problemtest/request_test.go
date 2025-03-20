package problemtest_test

import (
	"testing"

	"github.com/nickbryan/httputil/problem/problemtest"
)

func TestNewRequest(t *testing.T) {
	t.Parallel()

	t.Run("sets the url path to the instance URL", func(t *testing.T) {
		t.Parallel()

		req := problemtest.NewRequest("/some/instance/url")
		if req.URL.Path != "/some/instance/url" {
			t.Errorf("expected URL path to be: /some/instance/url, got: %s", req.URL.Path)
		}
	})
}
