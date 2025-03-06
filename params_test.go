package httputil_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/nickbryan/httputil"
)

func TestUnmarshalParams(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Sort      string    `query:"sort"`
		AuthToken string    `header:"Authorization"`
		Page      int       `query:"page"           default:"1"`
		IsActive  bool      `query:"is_active"      default:"false"`
		Price     float64   `query:"price"`
		ID        uuid.UUID `path:"id"`
		Unknown   string    `query:"unknown"` // Has no input value
		Untagged  string    // Untagged field, should be ignored
	}

	type emptyTagsStruct struct {
		UntaggedField string
	}

	testCases := map[string]struct {
		request     *http.Request
		output      any
		expected    any
		expectErr   bool
		expectedErr string
	}{
		"should extract query params, headers, and path variable correctly": {
			request: func() *http.Request {
				r := &http.Request{
					URL: &url.URL{
						RawQuery: "sort=asc&page=5&is_active=true&price=19.99&untagged=hello",
					},
					Header: http.Header{
						"Authorization": []string{"Bearer token"},
					},
				}
				r.SetPathValue("id", "123e4567-e89b-12d3-a456-426614174000")

				return r
			}(),
			output: &testStruct{},
			expected: &testStruct{
				Sort:      "asc",
				AuthToken: "Bearer token",
				Page:      5,
				IsActive:  true,
				Price:     19.99,
				ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
				Unknown:   "",
				Untagged:  "",
			},
			expectErr: false,
		},
		"should apply multiple default values when query params and headers are missing": {
			request: &http.Request{
				URL:    &url.URL{}, // Empty query
				Header: http.Header{},
			},
			output: &testStruct{},
			expected: &testStruct{
				Sort:      "",
				AuthToken: "",
				Page:      1,     // Default value
				IsActive:  false, // Default value
				Price:     0,     // Default value
				ID:        uuid.UUID{},
				Unknown:   "",
			},
			expectErr: false,
		},
		"should fail when attempting to unmarshal into unsupported field type": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "unsupported=value",
				},
			},
			output: &struct {
				Unsupported []int `query:"unsupported"`
			}{},
			expectErr:   true,
			expectedErr: "setting field value: unsupported field type: []int",
		},
		"should ignore untagged fields in the struct": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "untagged_field=value", // Shouldn't match
				},
			},
			output: &emptyTagsStruct{},
			expected: &emptyTagsStruct{
				UntaggedField: "", // Should remain unchanged
			},
			expectErr: false,
		},
		"should fail when the value type is valid but cannot be parsed (float64)": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "price=invalid", // Price is supposed to be float64
				},
			},
			output:      &testStruct{},
			expectErr:   true,
			expectedErr: `setting field value: failed to convert parameter "price" to float64: strconv.ParseFloat: parsing "invalid": invalid syntax`,
		},
		"should fail when the value type is valid but cannot be parsed (int)": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "page=invalid", // Page is supposed to be int
				},
			},
			output:      &testStruct{},
			expectErr:   true,
			expectedErr: `setting field value: failed to convert parameter "page" to int: strconv.Atoi: parsing "invalid": invalid syntax`,
		},
		"should fail when the value type is valid but cannot be parsed (bool)": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "is_active=notabool", // Active is supposed to be bool
				},
			},
			output:      &testStruct{},
			expectErr:   true,
			expectedErr: `setting field value: failed to convert parameter "is_active" to bool: strconv.ParseBool: parsing "notabool": invalid syntax`,
		},

		"should fail gracefully when an invalid UUID is provided": {
			request: func() *http.Request {
				r := &http.Request{
					URL: &url.URL{RawQuery: ""},
				}
				r.SetPathValue("id", "invalid-uuid")

				return r
			}(),
			output:      &testStruct{},
			expectErr:   true,
			expectedErr: `setting field value: failed to convert parameter "id" to uuid.UUID: invalid UUID length: 12`,
		},
		"should not fail when no value is present for a required parameter": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "", // Empty raw query
				},
			},
			output: &testStruct{},
			expected: &testStruct{
				Sort:      "",
				Page:      1,
				IsActive:  false,
				AuthToken: "",
				Price:     0,
			},
			expectErr: false,
		},
		"should fail when output is not a pointer to a struct": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "user_id=123",
				},
			},
			output:      struct{}{},
			expectErr:   true,
			expectedErr: "validating output type: output must be a pointer to a struct, got struct {}",
		},
		"should leave untouched fields when they don't match tags": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "nonexistent=5", // `nonexistent` tag does not exist in the struct
				},
			},
			output: &testStruct{},
			expected: &testStruct{
				Sort:      "",    // Should remain empty
				Page:      1,     // Default value
				IsActive:  false, // Default value
				Price:     0,     // Default for float64
				AuthToken: "",
				ID:        uuid.UUID{},
				Unknown:   "", // Untouched since no value matches
			},
			expectErr: false,
		},
		"should apply priority for query over defaults and ignore unused tags": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "page=10&ignored_tag=something",
				},
			},
			output: &testStruct{},
			expected: &testStruct{
				Sort:      "",
				Page:      10,    // Overridden by query
				IsActive:  false, // Default value remains
				Price:     0,     // Default remains
				AuthToken: "",
				ID:        uuid.UUID{}, // Default remains
			},
			expectErr: false,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			err := httputil.UnmarshalParams(testCase.request, testCase.output)

			if testCase.expectErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}

				if err.Error() != testCase.expectedErr {
					t.Fatalf("unexpected error message: got %q, want %q", err.Error(), testCase.expectedErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !cmp.Equal(testCase.output, testCase.expected) {
				t.Errorf("unexpected output: got %+v, want %+v", testCase.output, testCase.expected)
			}
		})
	}
}
