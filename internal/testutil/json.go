package testutil

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-cmp/cmp"
)

func DiffJSON(x, y string) string {
	return cmp.Diff(x, y, cmp.FilterValues(isValidJSON, cmp.Transformer("JSON", asJSON)))
}

func isValidJSON(x, y []byte) bool {
	return json.Valid(x) && json.Valid(y)
}

func asJSON(in []byte) any {
	var out any

	if err := json.Unmarshal(in, &out); err != nil {
		return fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return out
}
