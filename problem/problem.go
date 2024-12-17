// Package problem provides utilities for constructing and handling error responses
// in accordance with RFC 9457 (Problem Details for HTTP APIs).
//
// This package offers a structured way to create detailed error responses.
// Additionally, it provides helper functions for common error scenarios.
package problem

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// DefaultErrorDocumentationLocation is the default URL pointing to the documentation
	// for Problem Details format. It can be overridden using the ErrorDocumentationLocation var.
	DefaultErrorDocumentationLocation = "https://pkg.go.dev/github.com/nickbryan/httputil/problem#"
)

// ErrorDocumentationLocation specifies the URL for the documentation of the Problem Details format.
// This variable can be customized to point to your own API documentation or a different reference.
var ErrorDocumentationLocation = DefaultErrorDocumentationLocation //nolint:gochecknoglobals // Global var improves API without degrading user experience.

// DetailedError encapsulates the fields required to respond with an error
// in accordance with RFC 9457 (Problem Details for HTTP APIs).
type DetailedError struct {
	// Type is a URI reference that identifies the specific problem.
	Type string
	// Title is a short, human-readable summary of the problem type.
	Title string
	// Detail is a human-readable explanation specific to the occurrence of the problem.
	Detail string
	// Status is the HTTP status code associated with the problem.
	Status int
	// Instance is a URI reference that identifies the specific occurrence of the problem.
	Instance string
	// ExtensionMembers is a key-value map for vendor-specific extension members.
	ExtensionMembers map[string]any
}

// WithDetail creates a new DetailedError instance with the provided detail message.
// It returns a copy of the original DetailedError with the updated Detail field.
func (d *DetailedError) WithDetail(detail string) *DetailedError {
	clone := *d
	clone.Detail = detail

	return &clone
}

// Error implements the `error` interface, allowing DetailedError objects to be used as errors.
func (d *DetailedError) Error() string { return fmt.Sprintf("%d %s: %s", d.Status, d.Title, d.Detail) }

// MarshalJSON implements the `json.Marshaler` interface for DetailedError.
// It marshals the DetailedError object into a JSON byte slice.
func (d *DetailedError) MarshalJSON() ([]byte, error) {
	deets := make(map[string]any)

	deets["type"] = d.Type
	deets["title"] = d.Title
	deets["detail"] = d.Detail
	deets["status"] = d.Status
	deets["instance"] = d.Instance

	for k, v := range d.ExtensionMembers {
		deets[k] = v
	}

	bytes, err := json.Marshal(deets)
	if err != nil {
		return nil, fmt.Errorf("marshaling DetailedError as JSON: %w", err)
	}

	return bytes, nil
}

// Field represents a specific field that caused a violation constraint.
// It includes details about the error and a pointer to the field in the request body.
type Field struct {
	Detail  string `json:"detail"`
	Pointer string `json:"pointer"`
}

// ConstraintViolation creates a DetailedError for constraint violation errors.
// The Field describe the specific fields that violated constraints.
func ConstraintViolation(r *http.Request, fields ...Field) *DetailedError {
	return &DetailedError{
		Type:             DefaultErrorDocumentationLocation + "ConstraintViolation",
		Title:            "Constraint Violation",
		Detail:           "The request data violated one or more validation constraints",
		Status:           http.StatusBadRequest,
		Instance:         r.URL.Path,
		ExtensionMembers: map[string]any{"violations": fields},
	}
}

// BadRequest creates a DetailedError for bad request errors.
func BadRequest(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             DefaultErrorDocumentationLocation + "BadRequest",
		Title:            "Bad Request",
		Detail:           "The request is invalid or malformed",
		Status:           http.StatusBadRequest,
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}

// ServerError creates a DetailedError for internal server errors.
func ServerError(r *http.Request) *DetailedError {
	return &DetailedError{
		Type:             DefaultErrorDocumentationLocation + "ServerError",
		Title:            "Server Error",
		Detail:           "The server encountered an unexpected internal error",
		Status:           http.StatusInternalServerError,
		Instance:         r.URL.Path,
		ExtensionMembers: nil,
	}
}
