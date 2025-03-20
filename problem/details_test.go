package problem_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/nickbryan/httputil/problem"
)

func TestDetailedErrorMarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("returns an error if JSON marshal fails", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).WithExtension("k", make(chan any))

		_, err := badRequest.MarshalJSON()
		if err == nil {
			t.Fatal("want error, got nil")
		}

		if diff := cmp.Diff(err.Error(), "marshaling DetailedError as JSON: json: unsupported type: chan interface {}"); diff != "" {
			t.Errorf("error does not match expected:\n%s", diff)
		}
	})

	t.Run("formats the error string", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests"))

		if diff := cmp.Diff(badRequest.Error(), "400 Bad Request: The request is invalid or malformed"); diff != "" {
			t.Errorf("error does not match expected:\n%s", diff)
		}
	})

	testDetailedErrorMarshalJSON(t, map[string]detailedErrorTestCase{
		"updates the detail field when WithDetail is called": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).
					WithDetail("This is the overridden detail")
			},
			want: details{
				detail:         "This is the overridden detail",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-01",
				title:          "Bad Request",
				typeIdentifier: "bad-request",
				extensions:     "",
			},
		},
		"takes the last call to WithDetail into account when multiple calls are made": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).
					WithDetail("This is the overridden detail").
					WithDetail("This is the other overridden detail").
					WithDetail("This is the final overridden detail")
			},
			want: details{
				detail:         "This is the final overridden detail",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-01",
				title:          "Bad Request",
				typeIdentifier: "bad-request",
				extensions:     "",
			},
		},
		"adds extensions to the problem details when WithExtension is called": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).
					WithExtension("validation", "error")
			},
			want: details{
				detail:         "The request is invalid or malformed",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-01",
				title:          "Bad Request",
				typeIdentifier: "bad-request",
				extensions:     `,"validation":"error"`,
			},
		},
		"takes the last call to WithExtension into account when multiple calls are made with the same key": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).
					WithExtension("validation", "error").
					WithExtension("validation", "error2")
			},
			want: details{
				detail:         "The request is invalid or malformed",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-01",
				title:          "Bad Request",
				typeIdentifier: "bad-request",
				extensions:     `,"validation":"error2"`,
			},
		},
		"the base values can not be overridden by extensions": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).
					WithExtension("detail", "detail-override").
					WithExtension("instance", "instance-override").
					WithExtension("status", "status-override").
					WithExtension("code", "code-override").
					WithExtension("title", "title-override").
					WithExtension("type", "typeIdentifier-override")
			},
			want: details{
				detail:         "The request is invalid or malformed",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-01",
				title:          "Bad Request",
				typeIdentifier: "bad-request",
				extensions:     "",
			},
		},
	})
}

func TestDetailedErrorMustMarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("panics if JSON marshal fails", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).WithExtension("k", make(chan any))

		defer func(t *testing.T) {
			t.Helper()

			if r := recover(); r == nil {
				t.Fatal("want panic, got nil")
			} else {
				err, ok := r.(error)
				if !ok {
					t.Fatalf("unable to convert recover to error, got: %+v", r)
				}

				if diff := cmp.Diff(err.Error(), "marshaling DetailedError as JSON: json: unsupported type: chan interface {}"); diff != "" {
					t.Errorf("error does not match expected: \n%s", diff)
				}
			}
		}(t)

		badRequest.MustMarshalJSON()
	})

	t.Run("marshals valid JSON", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests"))

		defer func(t *testing.T) {
			t.Helper()

			if r := recover(); r != nil {
				t.Fatalf("unexpected panic: %v", r)
			}
		}(t)

		bytes := badRequest.MustMarshalJSON()
		if diff := cmp.Diff(string(bytes), `{"code":"400-01","detail":"The request is invalid or malformed","instance":"/tests","status":400,"title":"Bad Request","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md"}`); diff != "" {
			t.Errorf("bytes does not match expected:\n%s", diff)
		}
	})
}

func TestDetailedErrorMustMarshalJSONString(t *testing.T) {
	t.Parallel()

	t.Run("panics if JSON marshal fails", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).WithExtension("k", make(chan any))

		defer func(t *testing.T) {
			t.Helper()

			if r := recover(); r == nil {
				t.Fatal("want panic, got nil")
			} else {
				err, ok := r.(error)
				if !ok {
					t.Fatalf("unable to convert recover to error, got: %+v", r)
				}

				if diff := cmp.Diff(err.Error(), "marshaling DetailedError as JSON: json: unsupported type: chan interface {}"); diff != "" {
					t.Errorf("error does not match expected: \n%s", diff)
				}
			}
		}(t)

		badRequest.MustMarshalJSONString()
	})

	t.Run("marshals valid JSON as a string", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests"))

		defer func(t *testing.T) {
			t.Helper()

			if r := recover(); r != nil {
				t.Fatalf("unexpected panic: %v", r)
			}
		}(t)

		str := badRequest.MustMarshalJSONString()
		if diff := cmp.Diff(str, `{"code":"400-01","detail":"The request is invalid or malformed","instance":"/tests","status":400,"title":"Bad Request","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/bad-request.md"}`); diff != "" {
			t.Errorf("bytes does not match expected:\n%s", diff)
		}
	})
}

func TestDetailedErrorUnmarshalJSON(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		input         string
		want          *problem.DetailedError
		wantErr       bool
		wantErrString string
	}{
		"successfully unmarshals basic fields": {
			input: `{
                "type": "test-type",
                "title": "Test Title",
                "detail": "Test Detail",
                "status": 400,
                "code": "TEST-001",
                "instance": "/test/path"
            }`,
			want: &problem.DetailedError{
				Type:             "test-type",
				Title:            "Test Title",
				Detail:           "Test Detail",
				Status:           400,
				Code:             "TEST-001",
				Instance:         "/test/path",
				ExtensionMembers: map[string]any{},
			},
		},
		"successfully unmarshals with extension members": {
			input: `{
                "type": "test-type",
                "title": "Test Title",
                "detail": "Test Detail",
                "status": 400,
                "code": "TEST-001",
                "instance": "/test/path",
                "custom_field": "custom value",
                "severity": "high"
            }`,
			want: &problem.DetailedError{
				Type:     "test-type",
				Title:    "Test Title",
				Detail:   "Test Detail",
				Status:   400,
				Code:     "TEST-001",
				Instance: "/test/path",
				ExtensionMembers: map[string]any{
					"custom_field": "custom value",
					"severity":     "high",
				},
			},
		},
		"handles empty JSON object": {
			input: "{}",
			want: &problem.DetailedError{
				ExtensionMembers: map[string]any{},
			},
		},
		"returns error for invalid JSON": {
			input:         `{"type": "test-type"`,
			wantErr:       true,
			wantErrString: "unmarshaling DetailedError known fields: unexpected end of JSON input",
		},
		"handles null values": {
			input: `{
                "type": null,
                "title": null,
                "detail": null,
                "status": null,
                "code": null,
                "instance": null
            }`,
			want: &problem.DetailedError{
				ExtensionMembers: map[string]any{},
			},
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			var got problem.DetailedError
			err := got.UnmarshalJSON([]byte(testCase.input))

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}

				if diff := cmp.Diff(err.Error(), testCase.wantErrString); diff != "" {
					t.Errorf("error mismatch (-got +want):\n%s", diff)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(testCase.want, &got); diff != "" {
				t.Errorf("DetailedError mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type detailedErrorTestCase struct {
	newDetailedError func(t *testing.T) *problem.DetailedError
	want             details
}

type details struct {
	detail         string
	instance       string
	status         int
	code           string
	title          string
	typeIdentifier string
	extensions     string
}

func newRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, "http://localhost"+path, nil)
	if err != nil {
		t.Fatalf("unable to create request object: %+v", err)
	}

	return req
}

func testDetailedErrorMarshalJSON(t *testing.T, testCases map[string]detailedErrorTestCase) {
	t.Helper()

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			want := fmt.Sprintf(
				`{"code":"%s","detail":"%s","instance":"%s","status":%d,"title":"%s","type":"https://github.com/nickbryan/httputil/blob/main/docs/problems/%s.md"%s}`,
				testCase.want.code,
				testCase.want.detail,
				testCase.want.instance,
				testCase.want.status,
				testCase.want.title,
				testCase.want.typeIdentifier,
				testCase.want.extensions,
			)

			got, err := json.Marshal(testCase.newDetailedError(t))
			if err != nil {
				t.Fatalf("unable to marshal detailedError: %+v", err)
			}

			if diff := cmp.Diff(want, string(got)); diff != "" {
				t.Errorf("detailedError does not match expected:\n%s", diff)
			}
		})
	}
}
