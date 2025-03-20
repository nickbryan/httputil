package testutil_test

import (
	"testing"

	"github.com/nickbryan/httputil/internal/testutil"
)

func TestDiffJSON(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		x        string
		y        string
		wantDiff bool
	}{
		"identical JSON objects does not produce a diff": {
			x:        `{"name": "Alice", "age": 30}`,
			y:        `{"name": "Alice", "age": 30}`,
			wantDiff: false,
		},
		"different JSON objects produces a diff": {
			x:        `{"name": "Alice", "age": 30}`,
			y:        `{"name": "Bob", "age": 25}`,
			wantDiff: true,
		},
		"identical but formatted differently does not produce a diff": {
			x:        `{"name":"Alice","age":30}`,
			y:        "{\n  \"name\": \"Alice\",\n  \"age\": 30\n}",
			wantDiff: false,
		},
		"missing fields in one JSON produces a diff": {
			x:        `{"name": "Alice", "age": 30}`,
			y:        `{"name": "Alice"}`,
			wantDiff: true,
		},
		"extra fields in one JSON produces a diff": {
			x:        `{"name": "Alice"}`,
			y:        `{"name": "Alice", "city": "Wonderland"}`,
			wantDiff: true,
		},
		"arrays with identical elements does not produce a diff": {
			x:        `{"items": [1, 2, 3]}`,
			y:        `{"items": [1, 2, 3]}`,
			wantDiff: false,
		},
		"arrays with different elements produce a diff": {
			x:        `{"items": [1, 2, 3]}`,
			y:        `{"items": [1, 2, 4]}`,
			wantDiff: true,
		},
		"empty JSON objects do not produce a diff": {
			x:        `{}`,
			y:        `{}`,
			wantDiff: false,
		},
		"empty versus non-empty JSON produces a diff": {
			x:        `{}`,
			y:        `{"key": "value"}`,
			wantDiff: true,
		},
		"invalid JSON format produces a diff": {
			x:        `{"name": "Alice", "age": 30`,
			y:        `{"name": "Alice", "age": 30}`,
			wantDiff: true,
		},
		"identical JSON objects with additional whitespace does not produce a diff": {
			x:        `        {"name": "Alice", "age": 30}`,
			y:        `{"name": "Alice", "age": 30}        `,
			wantDiff: false,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			diff := testutil.DiffJSON(testCase.x, testCase.y)

			if diff == "" && testCase.wantDiff {
				t.Error("want diff but got none")
			}

			if diff != "" && !testCase.wantDiff {
				t.Errorf("got diff but want none, diff: \n%s", diff)
			}
		})
	}
}
