package problem

import (
	"net/http"
)

const (
	// DefaultErrorDocumentationLocation is the default URL pointing to the documentation
	// for Problem Details format. It can be overridden using the ErrorDocumentationLocation var.
	DefaultErrorDocumentationLocation = "https://github.com/nickbryan/httputil/blob/main/docs/problems/"
)

// ErrorDocumentationLocation specifies the URL for the documentation of the
// Problem Details format. This variable can be customized to point to your own
// API documentation or a different reference.
var ErrorDocumentationLocation = DefaultErrorDocumentationLocation //nolint:gochecknoglobals // Global var improves API without degrading user experience.

// ParameterType defines the type of the parameter that caused an error.
// It is used to classify parameters into query parameters, header parameters,
// or path parameters and to provide more context about the specific issue.
type ParameterType string

const (

	// ParameterTypeQuery indicates that the parameter error is related to a query
	// parameter. Query parameters are typically part of the URL and are used to
	// pass data to the server.
	ParameterTypeQuery ParameterType = "query"

	// ParameterTypeHeader indicates that the parameter error is related to a header
	// parameter. Header parameters are sent as part of the HTTP request headers and
	// provide metadata about the request or additional information required by the
	// server.
	ParameterTypeHeader ParameterType = "header"

	// ParameterTypePath indicates that the parameter error is related to a path
	// parameter. Path parameters are used in the URL path and typically represent a
	// resource identifier or dynamic data.
	ParameterTypePath ParameterType = "path"
)

// Parameter represents a specific parameter that caused an error during request
// validation. It provides details about the error, the parameter name, and its
// type (query, header, path).
type Parameter struct {
	Parameter string        `json:"parameter"`
	Detail    string        `json:"detail"`
	Type      ParameterType `json:"type"`
}

// Property represents a specific property that caused a violation constraint. It
// includes details about the error and a pointer to the field in the request
// body.
type Property struct {
	Detail  string `json:"detail"`
	Pointer string `json:"pointer"`
}

// BadParameters creates a DetailedError for invalid or malformed request
// parameters. This function is used when the request contains query, header, or
// path parameters that do not meet the expected requirements. You may provide
// details about the specific violations using the parameters parameter. Each
// parameter in the parameters slice holds information about the invalid
// parameter, including its name, type, and a description of the issue.
func BadParameters(r *http.Request, parameters ...Parameter) *DetailedError {
	if parameters == nil {
		parameters = []Parameter{}
	}

	return &DetailedError{
		Type:             typeLocation("bad-parameters"),
		Title:            "Bad Parameters",
		Detail:           "The request parameters are invalid or malformed",
		Status:           http.StatusBadRequest,
		Code:             "400-02",
		Instance:         r.URL.Path,
		ExtensionMembers: map[string]any{"violations": parameters},
	}
}

// BadRequest creates a DetailedError for bad request errors.
func BadRequest(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             typeLocation("bad-request"),
		Title:            "Bad Request",
		Detail:           "The request is invalid or malformed",
		Status:           http.StatusBadRequest,
		Code:             "400-01",
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}

// BusinessRuleViolation creates a DetailedError for business rule violation errors.
// This function is used when a request violates one or more business rules set by
// the application. You may pass additional details about the specific violations
// using the properties parameter.
func BusinessRuleViolation(r *http.Request, properties ...Property) *DetailedError {
	if properties == nil {
		properties = []Property{}
	}

	return &DetailedError{
		Type:             typeLocation("business-rule-violation"),
		Title:            "Business Rule Violation",
		Detail:           "The request violates one or more business rules",
		Status:           http.StatusUnprocessableEntity,
		Code:             "422-01",
		Instance:         r.URL.Path,
		ExtensionMembers: map[string]any{"violations": properties},
	}
}

// ConstraintViolation creates a DetailedError for constraint validation errors.
// This function is used when the request data violates one or more validation
// constraints. You may provide additional details about the specific violations
// using the properties parameter.
// If no properties are provided, the violations field will be an empty array.
func ConstraintViolation(r *http.Request, properties ...Property) *DetailedError {
	if properties == nil {
		properties = []Property{}
	}

	return &DetailedError{
		Type:             typeLocation("constraint-violation"),
		Title:            "Constraint Violation",
		Detail:           "The request data violated one or more validation constraints",
		Status:           http.StatusUnprocessableEntity,
		Code:             "422-02",
		Instance:         r.URL.Path,
		ExtensionMembers: map[string]any{"violations": properties},
	}
}

// Forbidden creates a DetailedError for forbidden errors.
func Forbidden(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             typeLocation("forbidden"),
		Title:            "Forbidden",
		Detail:           "You do not have the necessary permissions to " + r.Method + " this resource",
		Status:           http.StatusForbidden,
		Code:             "403-01",
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}

// NotFound creates a DetailedError for not found errors.
func NotFound(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             typeLocation("not-found"),
		Title:            "Not Found",
		Detail:           "The requested resource was not found",
		Status:           http.StatusNotFound,
		Code:             "404-01",
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}

// ResourceExists creates a DetailedError for duplicate resource errors.
func ResourceExists(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             typeLocation("resource-exists"),
		Title:            "Resource Exists",
		Detail:           "A resource already exists with the specified identifier",
		Status:           http.StatusConflict,
		Code:             "409-01",
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}

// ServerError creates a DetailedError for internal server errors.
func ServerError(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             typeLocation("server-error"),
		Title:            "Server Error",
		Detail:           "The server encountered an unexpected internal error",
		Status:           http.StatusInternalServerError,
		Code:             "500-01",
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}

// Unauthorized creates a DetailedError for unauthorized errors.
func Unauthorized(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             typeLocation("unauthorized"),
		Title:            "Unauthorized",
		Detail:           "You must be authenticated to " + r.Method + " this resource",
		Status:           http.StatusUnauthorized,
		Code:             "401-01",
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}

func typeLocation(t string) string {
	return ErrorDocumentationLocation + t + ".md"
}
