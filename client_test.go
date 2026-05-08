package httputil_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"
)

func TestClient_Do(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(server.Close)

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
	client := httputil.NewClient(logger)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/test", http.NoBody)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response body: %s", err)
		}
	})

	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, resp.StatusCode)
	}
}

func TestClient_BasePath(t *testing.T) {
	t.Parallel()

	logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)

	testCases := map[string]struct {
		basePath string
		want     string
	}{
		"returns empty string when not set": {
			basePath: "",
			want:     "",
		},
		"returns base path as configured": {
			basePath: "https://example.com/api",
			want:     "https://example.com/api",
		},
		"trims trailing slash": {
			basePath: "https://example.com/api/",
			want:     "https://example.com/api",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := httputil.NewClient(logger, httputil.WithClientBasePath(tc.basePath))
			if got := client.BasePath(); got != tc.want {
				t.Errorf("BasePath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClient_Codec(t *testing.T) {
	t.Parallel()

	t.Run("defaults to JSONClientCodec", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		client := httputil.NewClient(logger)

		if client.Codec().ContentType() != "application/json; charset=utf-8" {
			t.Errorf("expected default codec content type to be application/json; charset=utf-8, got: %s", client.Codec().ContentType())
		}
	})

	t.Run("uses custom codec when set", func(t *testing.T) {
		t.Parallel()

		logger, _ := slogutil.NewInMemoryLogger(slog.LevelDebug)
		custom := &fakeClientCodec{contentType: "application/xml"}
		client := httputil.NewClient(logger, httputil.WithClientCodec(custom))

		if client.Codec().ContentType() != "application/xml" {
			t.Errorf("expected codec content type to be application/xml, got: %s", client.Codec().ContentType())
		}
	})
}

type fakeClientCodec struct {
	contentType string
	encode      func(any) (io.Reader, error)
	decode      func(io.Reader, any) error
}

func (f *fakeClientCodec) ContentType() string {
	return f.contentType
}

func (f *fakeClientCodec) Encode(data any) (io.Reader, error) {
	if f.encode != nil {
		return f.encode(data)
	}

	return nil, nil
}

func (f *fakeClientCodec) Decode(r io.Reader, into any) error {
	if f.decode != nil {
		return f.decode(r, into)
	}

	return nil
}
