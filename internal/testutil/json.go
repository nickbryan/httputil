// Package testutil provides helper functions for writing tests.
package testutil

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// DiffJSON compares two JSON strings and returns a diff string highlighting the differences.
// Invalid whitespace within JSON strings is removed before comparison.
func DiffJSON(x, y string) string {
	return cmp.Diff(compactJSON(x), compactJSON(y))
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
