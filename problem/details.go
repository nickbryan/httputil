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
	Type string `json:"type"`
	// Title is a short, human-readable summary of the problem type.
	Title string `json:"title"`
	// Detail is a human-readable explanation specific to the occurrence of the problem.
	Detail string `json:"detail"`
	// Status is the HTTP status code associated with the problem.
	Status int `json:"status"`
	// Code is the domain-specific error code associated with the problem.
	Code string `json:"code"`
	// Instance is a URI reference that identifies the specific occurrence of the problem.
	Instance string `json:"instance"`
	// ExtensionMembers is a key-value map for vendor-specific extension members.
	ExtensionMembers map[string]any `json:"-"` // See DetailedError.UnmarshalJSON for how this is mapped.
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
	fields := make(map[string]any)

	for k, v := range d.ExtensionMembers {
		fields[k] = v
	}

	fields["type"] = d.Type
	fields["title"] = d.Title
	fields["detail"] = d.Detail
	fields["status"] = d.Status
	fields["code"] = d.Code
	fields["instance"] = d.Instance

	bytes, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("marshaling DetailedError as JSON: %w", err)
	}

	return bytes, nil
}

// MustMarshalJSON marshals the DetailedError into JSON and panics if an error
// occurs during the marshaling process. This is useful for testing the
// comparison of error responses.
func (d *DetailedError) MustMarshalJSON() []byte {
	bytes, err := d.MarshalJSON()
	if err != nil {
		panic(err)
	}

	return bytes
}

// MustMarshalJSONString converts the DetailedError to a JSON string and panics
// if an error occurs during marshaling.
func (d *DetailedError) MustMarshalJSONString() string {
	return string(d.MustMarshalJSON())
}

// UnmarshalJSON implements the `json.Unmarshaler` interface for DetailedError,
// handling known and unknown fields gracefully.
func (d *DetailedError) UnmarshalJSON(data []byte) error {
	var known struct {
		Type     string `json:"type"`
		Title    string `json:"title"`
		Detail   string `json:"detail"`
		Status   int    `json:"status"`
		Code     string `json:"code"`
		Instance string `json:"instance"`
	}

	if err := json.Unmarshal(data, &known); err != nil {
		return fmt.Errorf("unmarshaling DetailedError known fields: %w", err)
	}

	*d = DetailedError{
		Type:             known.Type,
		Title:            known.Title,
		Detail:           known.Detail,
		Status:           known.Status,
		Code:             known.Code,
		Instance:         known.Instance,
		ExtensionMembers: nil,
	}

	if err := json.Unmarshal(data, &d.ExtensionMembers); err != nil {
		// This shouldn't every happen because the first unmarshal will fail before this gets called.
		return fmt.Errorf("unmarshaling DetailedError extension members: %w", err)
	}

	maps.DeleteFunc(d.ExtensionMembers, func(k string, _ any) bool {
		return k == "type" || k == "title" || k == "detail" || k == "status" || k == "code" || k == "instance"
	})

	return nil
}
