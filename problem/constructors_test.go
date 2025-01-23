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

func TestConstructors(t *testing.T) {
	t.Parallel()

	newRequest := func(method, path string) *http.Request {
		req, err := http.NewRequestWithContext(context.Background(), method, "http://localhost"+path, nil)
		if err != nil {
			t.Fatalf("unable to create request object: %+v", err)
		}

		return req
	}

	type details struct {
		detail         string
		instance       string
		status         int
		title          string
		typeIdentifier string
		extensions     string
	}

	testCases := map[string]struct {
		detailedError *problem.DetailedError
		want          details
	}{
		"bad request sets the expected problem details for the resource instance": {
			detailedError: problem.BadRequest(newRequest(http.MethodGet, "/tests")),
			want: details{
				detail:         "The request is invalid or malformed",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				title:          "Bad Request",
				typeIdentifier: "BadRequest",
				extensions:     "",
			},
		},
		"constraint violation sets the expected problem details for the resource instance when no fields are passed": {
			detailedError: problem.ConstraintViolation(newRequest(http.MethodGet, "/tests")),
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				title:          "Constraint Violation",
				typeIdentifier: "ConstraintViolation",
				extensions:     `,"violations":[]`,
			},
		},
		"constraint violation sets the expected problem details for the resource instance when a single field is passed": {
			detailedError: problem.ConstraintViolation(newRequest(http.MethodGet, "/tests"), problem.Field{
				Detail:  "Invalid",
				Pointer: "/",
			}),
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				title:          "Constraint Violation",
				typeIdentifier: "ConstraintViolation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"/"}]`,
			},
		},
		"constraint violation sets the expected problem details for the resource instance when multiple fields are passed": {
			detailedError: problem.ConstraintViolation(
				newRequest(http.MethodGet, "/tests"),
				problem.Field{Detail: "Invalid", Pointer: "/thing"},
				problem.Field{Detail: "Short", Pointer: "/other"},
			),
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				title:          "Constraint Violation",
				typeIdentifier: "ConstraintViolation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"/thing"},{"detail":"Short","pointer":"/other"}]`,
			},
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			want := fmt.Sprintf(
				`{"detail":"%s","instance":"%s","status":%d,"title":"%s","type":"https://pkg.go.dev/github.com/nickbryan/httputil/problem#%s"%s}`,
				testCase.want.detail,
				testCase.want.instance,
				testCase.want.status,
				testCase.want.title,
				testCase.want.typeIdentifier,
				testCase.want.extensions,
			)

			got, err := json.Marshal(testCase.detailedError)
			if err != nil {
				t.Fatalf("unable to marshal detailedError: %+v", err)
			}

			if diff := cmp.Diff(want, string(got)); diff != "" {
				t.Errorf("detailedError does not match expected:\v%s", diff)
			}
		})
	}
}
