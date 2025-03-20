// Package testutil provides helper functions for writing tests.
package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// DiffJSON compares two JSON strings and returns a diff string highlighting the differences.
// Invalid whitespace within JSON strings is removed before comparison.
func DiffJSON(x, y string) string {
	return cmp.Diff(
		compactJSON(x),
		compactJSON(y),
		cmp.FilterValues(isValidJSON, cmp.Transformer("JSON", asJSON)),
	)
}

// compactJSON removes whitespace from a JSON string.
// If the input is not valid JSON, it returns the original string trimmed.
func compactJSON(in string) string {
	trimmed := strings.TrimSpace(in)
	if !json.Valid([]byte(trimmed)) {
		return trimmed
	}

	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(trimmed)); err != nil {
		return trimmed
	}

	return buf.String()
}

// isValidJSON checks if both provided byte slices represent valid JSON data.
func isValidJSON(x, y []byte) bool {
	return json.Valid(x) && json.Valid(y)
}

// asJSON unmarshals the provided byte slice into an any type. It attempts to
// decode the JSON and returns a wrapped error if the unmarshalling fails.
func asJSON(in []byte) any {
	var out any

	if err := json.Unmarshal(in, &out); err != nil {
		return fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return out
}
