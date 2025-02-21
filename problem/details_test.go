package problem_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/nickbryan/httputil/problem"
)

func TestDetailedError(t *testing.T) {
	t.Parallel()

	t.Run("returns an error if JSON marshal fails", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests")).WithExtension("k", make(chan any))

		_, err := badRequest.MarshalJSON()
		if err == nil {
			t.Fatal("want error, got nil")
		}

		if diff := cmp.Diff(err.Error(), "marshaling DetailedError as JSON: json: unsupported type: chan interface {}"); diff != "" {
			t.Errorf("error does not match expected:\v%s", diff)
		}
	})

	t.Run("formats the error string", func(t *testing.T) {
		t.Parallel()

		badRequest := problem.BadRequest(newRequest(t, http.MethodGet, "/tests"))

		if diff := cmp.Diff(badRequest.Error(), "400 Bad Request: The request is invalid or malformed"); diff != "" {
			t.Errorf("error does not match expected:\v%s", diff)
		}
	})

	testDetailedError(t, map[string]detailedErrorTestCase{
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

	req, err := http.NewRequestWithContext(context.Background(), method, "http://localhost"+path, nil)
	if err != nil {
		t.Fatalf("unable to create request object: %+v", err)
	}

	return req
}

func testDetailedError(t *testing.T, testCases map[string]detailedErrorTestCase) {
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
				t.Errorf("detailedError does not match expected:\v%s", diff)
			}
		})
	}
}
