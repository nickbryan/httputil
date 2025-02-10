// Package problem provides utilities for constructing and handling error
// responses in accordance with RFC 9457 (Problem Details for HTTP APIs).
//
// This package offers a structured way to create detailed error responses.
// Additionally, it provides helper functions for common error scenarios.
package problem

import (
	"encoding/json"
	"fmt"
	"maps"
)

// DetailedError encapsulates the fields required to respond with an error in
// accordance with RFC 9457 (Problem Details for HTTP APIs).
type DetailedError struct {
	// Type is a URI reference that identifies the specific problem.
	Type string
	// Title is a short, human-readable summary of the problem type.
	Title string
	// Detail is a human-readable explanation specific to the occurrence of the problem.
	Detail string
	// Status is the HTTP status code associated with the problem.
	Status int
	// Code is the domain-specific error code associated with the problem.
	Code string
	// Instance is a URI reference that identifies the specific occurrence of the problem.
	Instance string
	// ExtensionMembers is a key-value map for vendor-specific extension members.
	ExtensionMembers map[string]any
}

// WithDetail creates a new DetailedError instance with the provided detail
// message. It returns a copy of the original DetailedError with the updated
// Detail field.
func (d *DetailedError) WithDetail(detail string) *DetailedError {
	clone := *d

	clone.Detail = detail
	clone.ExtensionMembers = maps.Clone(d.ExtensionMembers)

	return &clone
}

// WithExtension creates a new DetailedError instance with an added or updated
// extension member. It returns a copy of the original DetailedError with the
// specified extension member added or updated. If the original DetailedError has
// no ExtensionMembers, a new map is created. Returns a new DetailedError
// instance with the added or updated extension member. The original
// DetailedError is not modified.
func (d *DetailedError) WithExtension(k string, v any) *DetailedError {
	clone := *d

	clone.ExtensionMembers = maps.Clone(d.ExtensionMembers)
	if clone.ExtensionMembers == nil {
		clone.ExtensionMembers = make(map[string]any, 1)
	}

	clone.ExtensionMembers[k] = v

	return &clone
}

// Error implements the `error` interface, allowing DetailedError objects to be
// used as errors.
func (d *DetailedError) Error() string { return fmt.Sprintf("%d %s: %s", d.Status, d.Title, d.Detail) }

// MarshalJSON implements the `json.Marshaler` interface for DetailedError. It
// marshals the DetailedError object into a JSON byte slice.
func (d *DetailedError) MarshalJSON() ([]byte, error) {
	deets := make(map[string]any)

	for k, v := range d.ExtensionMembers {
		deets[k] = v
	}

	deets["type"] = d.Type
	deets["title"] = d.Title
	deets["detail"] = d.Detail
	deets["status"] = d.Status
	deets["code"] = d.Code
	deets["instance"] = d.Instance

	bytes, err := json.Marshal(deets)
	if err != nil {
		return nil, fmt.Errorf("marshaling DetailedError as JSON: %w", err)
	}

	return bytes, nil
}
