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
		code           string
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
				code:           "400-01",
				title:          "Bad Request",
				typeIdentifier: "bad-request",
				extensions:     "",
			},
		},
		"constraint violation sets the expected problem details for the resource instance when no fields are passed": {
			detailedError: problem.ConstraintViolation(newRequest(http.MethodGet, "/tests")),
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-02",
				title:          "Constraint Violation",
				typeIdentifier: "constraint-violation",
				extensions:     `,"violations":[]`,
			},
		},
		"constraint violation sets the expected problem details for the resource instance when a single field is passed": {
			detailedError: problem.ConstraintViolation(newRequest(http.MethodGet, "/tests"), problem.Property{
				Detail:  "Invalid",
				Pointer: "/",
			}),
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-02",
				title:          "Constraint Violation",
				typeIdentifier: "constraint-violation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"/"}]`,
			},
		},
		"constraint violation sets the expected problem details for the resource instance when multiple fields are passed": {
			detailedError: problem.ConstraintViolation(
				newRequest(http.MethodGet, "/tests"),
				problem.Property{Detail: "Invalid", Pointer: "/thing"},
				problem.Property{Detail: "Short", Pointer: "/other"},
			),
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-02",
				title:          "Constraint Violation",
				typeIdentifier: "constraint-violation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"/thing"},{"detail":"Short","pointer":"/other"}]`,
			},
		},
		"forbidden sets the expected problem details for the resource instance": {
			detailedError: problem.Forbidden(newRequest(http.MethodGet, "/forbidden")),
			want: details{
				detail:         "You do not have the necessary permissions to GET this resource",
				instance:       "/forbidden",
				status:         http.StatusForbidden,
				code:           "403-01",
				title:          "Forbidden",
				typeIdentifier: "forbidden",
				extensions:     "",
			},
		},
		"not found sets the expected problem details for the resource instance": {
			detailedError: problem.NotFound(newRequest(http.MethodGet, "/missing")),
			want: details{
				detail:         "The requested resource was not found",
				instance:       "/missing",
				status:         http.StatusNotFound,
				code:           "404-01",
				title:          "Not Found",
				typeIdentifier: "not-found",
				extensions:     "",
			},
		},

		"conflict sets the expected problem details for the resource instance": {
			detailedError: problem.ResourceExists(newRequest(http.MethodPost, "/conflict")),
			want: details{
				detail:         "A resource already exists with the specified identifier",
				instance:       "/conflict",
				status:         http.StatusConflict,
				code:           "409-01",
				title:          "Resource Exists",
				typeIdentifier: "resource-exists",
				extensions:     "",
			},
		},
		"internal server error sets the expected problem details for the resource instance": {
			detailedError: problem.ServerError(newRequest(http.MethodPost, "/error")),
			want: details{
				detail:         "The server encountered an unexpected internal error",
				instance:       "/error",
				status:         http.StatusInternalServerError,
				code:           "500-01",
				title:          "Server Error",
				typeIdentifier: "server-error",
				extensions:     "",
			},
		},
		"unauthorized sets the expected problem details for the resource instance": {
			detailedError: problem.Unauthorized(newRequest(http.MethodGet, "/private")),
			want: details{
				detail:         "You must be authenticated to GET this resource",
				instance:       "/private",
				status:         http.StatusUnauthorized,
				code:           "401-01",
				title:          "Unauthorized",
				typeIdentifier: "unauthorized",
				extensions:     "",
			},
		},
	}

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
