package problem_test

import (
	"net/http"
	"testing"

	"github.com/nickbryan/httputil/problem"
)

func TestConstructors(t *testing.T) {
	t.Parallel()

	testDetailedError(t, map[string]detailedErrorTestCase{
		"bad parameters sets the expected problem details for the resource instance when no fields are passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadParameters(newRequest(t, http.MethodGet, "/tests"))
			},
			want: details{
				detail:         "The request parameters are invalid or malformed",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-02",
				title:          "Bad Parameters",
				typeIdentifier: "bad-parameters",
				extensions:     `,"violations":[]`,
			},
		},
		"bad parameters sets the expected problem details for the resource instance when a single field is passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadParameters(newRequest(t, http.MethodGet, "/tests"), problem.Parameter{
					Parameter: "thing",
					Detail:    "Invalid",
					Type:      problem.ParameterTypeHeader,
				})
			},
			want: details{
				detail:         "The request parameters are invalid or malformed",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-02",
				title:          "Bad Parameters",
				typeIdentifier: "bad-parameters",
				extensions:     `,"violations":[{"parameter":"thing","detail":"Invalid","type":"header"}]`,
			},
		},
		"bad parameters sets the expected problem details for the resource instance when multiple fields are passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadParameters(
					newRequest(t, http.MethodGet, "/tests"),
					problem.Parameter{Detail: "Invalid", Parameter: "thing", Type: problem.ParameterTypeHeader},
					problem.Parameter{Detail: "Short", Parameter: "other", Type: problem.ParameterTypeQuery},
					problem.Parameter{Detail: "Missing", Parameter: "stuff", Type: problem.ParameterTypePath},
				)
			},
			want: details{
				detail:         "The request parameters are invalid or malformed",
				instance:       "/tests",
				status:         http.StatusBadRequest,
				code:           "400-02",
				title:          "Bad Parameters",
				typeIdentifier: "bad-parameters",
				extensions:     `,"violations":[{"parameter":"thing","detail":"Invalid","type":"header"},{"parameter":"other","detail":"Short","type":"query"},{"parameter":"stuff","detail":"Missing","type":"path"}]`,
			},
		},
		"bad request sets the expected problem details for the resource instance": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BadRequest(newRequest(t, http.MethodGet, "/tests"))
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
		"business rule violation sets the expected problem details for the resource instance when no fields are passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BusinessRuleViolation(newRequest(t, http.MethodGet, "/tests"))
			},
			want: details{
				detail:         "The request violates one or more business rules",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-01",
				title:          "Business Rule Violation",
				typeIdentifier: "business-rule-violation",
				extensions:     `,"violations":[]`,
			},
		},
		"business rule violation sets the expected problem details for the resource instance when a single field is passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BusinessRuleViolation(newRequest(t, http.MethodGet, "/tests"), problem.Property{
					Detail:  "Invalid",
					Pointer: "#/",
				})
			},
			want: details{
				detail:         "The request violates one or more business rules",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-01",
				title:          "Business Rule Violation",
				typeIdentifier: "business-rule-violation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"#/"}]`,
			},
		},
		"business rule violation sets the expected problem details for the resource instance when multiple fields are passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.BusinessRuleViolation(
					newRequest(t, http.MethodGet, "/tests"),
					problem.Property{Detail: "Invalid", Pointer: "#/thing"},
					problem.Property{Detail: "Short", Pointer: "#/other"},
				)
			},
			want: details{
				detail:         "The request violates one or more business rules",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-01",
				title:          "Business Rule Violation",
				typeIdentifier: "business-rule-violation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"#/thing"},{"detail":"Short","pointer":"#/other"}]`,
			},
		},
		"constraint violation sets the expected problem details for the resource instance when no fields are passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.ConstraintViolation(newRequest(t, http.MethodGet, "/tests"))
			},
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
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.ConstraintViolation(newRequest(t, http.MethodGet, "/tests"), problem.Property{
					Detail:  "Invalid",
					Pointer: "#/",
				})
			},
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-02",
				title:          "Constraint Violation",
				typeIdentifier: "constraint-violation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"#/"}]`,
			},
		},
		"constraint violation sets the expected problem details for the resource instance when multiple fields are passed": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.ConstraintViolation(
					newRequest(t, http.MethodGet, "/tests"),
					problem.Property{Detail: "Invalid", Pointer: "#/thing"},
					problem.Property{Detail: "Short", Pointer: "#/other"},
				)
			},
			want: details{
				detail:         "The request data violated one or more validation constraints",
				instance:       "/tests",
				status:         http.StatusUnprocessableEntity,
				code:           "422-02",
				title:          "Constraint Violation",
				typeIdentifier: "constraint-violation",
				extensions:     `,"violations":[{"detail":"Invalid","pointer":"#/thing"},{"detail":"Short","pointer":"#/other"}]`,
			},
		},
		"forbidden sets the expected problem details for the resource instance": {
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.Forbidden(newRequest(t, http.MethodGet, "/forbidden"))
			},
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
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.NotFound(newRequest(t, http.MethodGet, "/missing"))
			},
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
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.ResourceExists(newRequest(t, http.MethodPost, "/conflict"))
			},
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
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.ServerError(newRequest(t, http.MethodPost, "/error"))
			},
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
			newDetailedError: func(t *testing.T) *problem.DetailedError {
				t.Helper()
				return problem.Unauthorized(newRequest(t, http.MethodGet, "/private"))
			},
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
	})
}
