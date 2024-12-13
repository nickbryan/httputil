// Package testutil provides helper functions for writing tests.
package testutil

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
)

// DiffJSON compares two JSON strings and returns a diff string highlighting the differences.
func DiffJSON(x, y string) string {
	return cmp.Diff(x, y, cmp.FilterValues(isValidJSON, cmp.Transformer("JSON", asJSON)))
}

// isValidJSON checks if both provided byte slices represent valid JSON data.
func isValidJSON(x, y []byte) bool {
	return json.Valid(x) && json.Valid(y)
}

// asJSON unmarshals the provided byte slice into an any type. It attempts to decode the JSON
// and returns a wrapped error if the unmarshalling fails.
func asJSON(in []byte) any {
	var out any

	if err := json.Unmarshal(in, &out); err != nil {
		return fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return out
}
