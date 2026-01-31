package httputil_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/httputil/problem"
)

func TestBindValidParameters(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Sort      string    `param:"query=sort"`
		AuthToken string    `param:"header=Authorization"`
		Page      int       `param:"query=page,default=1"`
		IsActive  bool      `param:"query=is_active,default=false"`
		Price     float64   `param:"query=price"`
		ID        uuid.UUID `param:"path=id"`
		Unknown   string    `param:"query=unknown"` // Has no input value
		Untagged  string    // Untagged field, should be ignored
	}

	type validateTestStruct struct {
		RequiredString string `validate:"required" param:"query=required_string"`
	}

	type validateTestStructDefault struct {
		RequiredString string `validate:"required" param:"query=required_string,default=default"`
	}

	type emptyTagsStruct struct {
		UntaggedField string
	}

	type unexportedStruct struct {
		visible string `param:"query=visible"`
	}

	type fallbackStruct struct {
		Val string `validate:"min=5" param:"query=q,header=H"`
	}

	type fallbackTypeStruct struct {
		Val int `param:"query=q,header=H"`
	}

	type mixedErrorStruct struct {
		IntVal   int    `param:"query=ival"`
		StrVal   string `validate:"min=5"         param:"query=sval"`
		Fallback int    `param:"query=f,header=H"`
	}

	type defaultValidationStruct struct {
		Val string `validate:"min=5" param:"query=q,default=abc"`
	}

	type noParamTagStruct struct {
		Val string `validate:"required"`
	}

	type pureDefaultStruct struct {
		Val int `param:"default=123"`
	}

	type emptyValueStruct struct {
		Val string `param:"query=q"`
	}

	// Struct tags are included in the 'expected' struct literals to ensure they
	// match the type identity of the 'output' anonymous structs, as Go
	// considers tags part of the type.
	testCases := map[string]struct {
		request             *http.Request
		output              any
		expected            any
		expectErr           bool
		expectedErr         string
		expectedParamErrors []problem.Parameter
		cmpOptions          []cmp.Option
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
				Unsupported []int `param:"query=unsupported"`
			}{},
			expectErr:   true,
			expectedErr: "setting field value: unsupported field type: []int",
		},
		"should ignore untagged fields in the struct": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "UntaggedField=value",
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
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "price", Detail: "must be a valid float64", Type: problem.ParameterTypeQuery},
			},
		},
		"should fail when the value type is valid but cannot be parsed (int)": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "page=invalid", // Page is supposed to be int
				},
			},
			output:      &testStruct{},
			expectErr:   true,
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "page", Detail: "must be a valid int", Type: problem.ParameterTypeQuery},
			},
		},
		"should fail when the value type is valid but cannot be parsed (bool)": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "is_active=notabool", // Active is supposed to be bool
				},
			},
			output:      &testStruct{},
			expectErr:   true,
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "is_active", Detail: "must be a valid bool", Type: problem.ParameterTypeQuery},
			},
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
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "id", Detail: "must be a valid uuid.UUID", Type: problem.ParameterTypePath},
			},
		},
		"should not fail when no value is present": {
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
				Sort:      "",
				Page:      1,
				IsActive:  false,
				Price:     0,
				AuthToken: "",
				ID:        uuid.UUID{},
				Unknown:   "",
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
		"should return an error when the default value on a struct field is invalid for the type": {
			request: httptest.NewRequest(http.MethodGet, "/tests", nil),
			output: &struct {
				Test int `param:"default=not an int"`
			}{},
			expectErr:   true,
			expectedErr: `setting field value: failed to convert parameter "default" to int: strconv.Atoi: parsing "not an int": invalid syntax`,
		},
		"validates the struct fields when the validate tag is present": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "required_string=",
				},
			},
			output:      &validateTestStruct{},
			expectErr:   true,
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "required_string", Detail: "is required", Type: problem.ParameterTypeQuery},
			},
		},
		"passes validation when the validate tag is present and the field is valid": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "required_string=mystring",
				},
			},
			output: &validateTestStruct{},
			expected: &validateTestStruct{
				RequiredString: "mystring",
			},
		},
		"default value takes precedence over the validation when set": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "required_string=",
				},
			},
			output: &validateTestStructDefault{},
			expected: &validateTestStructDefault{
				RequiredString: "default",
			},
		},
		"should honor first match wins strategy": {
			request: func() *http.Request {
				r := &http.Request{
					URL: &url.URL{
						RawQuery: "q=queryValue",
					},
					Header: http.Header{
						"H": []string{"headerValue"},
					},
				}

				return r
			}(),
			output: &struct {
				// Header is first, should win
				Val1 string `param:"header=H,query=q"`
				// Query is first, should win
				Val2 string `param:"query=q,header=H"`
			}{},
			expected: &struct {
				Val1 string `param:"header=H,query=q"`
				Val2 string `param:"query=q,header=H"`
			}{
				Val1: "headerValue",
				Val2: "queryValue",
			},
			expectErr: false,
		},
		"should fall back to default when all other sources are missing": {
			request: &http.Request{
				URL:    &url.URL{},
				Header: http.Header{},
			},
			output: &struct {
				Val string `param:"query=q,header=h,default=fallback"`
			}{},
			expected: &struct {
				Val string `param:"query=q,header=h,default=fallback"`
			}{
				Val: "fallback",
			},
			expectErr: false,
		},
		"should find first available value in a chain (skipping missing ones)": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "q=queryVal",
				},
				Header: http.Header{}, // Empty header
			},
			output: &struct {
				Val string `param:"header=h,query=q,path=p"`
			}{},
			expected: &struct {
				Val string `param:"header=h,query=q,path=p"`
			}{
				Val: "queryVal",
			},
			expectErr: false,
		},
		"should handle complex source ordering correctly": {
			request: func() *http.Request {
				r := &http.Request{
					URL: &url.URL{
						RawQuery: "q=queryVal",
					},
					Header: http.Header{
						"H": []string{"headerVal"},
					},
				}
				r.SetPathValue("p", "pathVal")

				return r
			}(),
			output: &struct {
				// Order: Path -> Header -> Query -> Default
				// Path ("pathVal") is present, so it should win over Header ("headerVal")
				F1 string `param:"path=p,header=H,query=q,default=def"`

				// Order: Header -> Query -> Path -> Default
				// Header ("headerVal") is present, so it should win over Query ("queryVal")
				F2 string `param:"header=H,query=q,path=p,default=def"`

				// Order: Query -> Path -> Header -> Default
				// Query ("queryVal") is present, so it should win over Path ("pathVal")
				F3 string `param:"query=q,path=p,header=H,default=def"`
			}{},
			expected: &struct {
				F1 string `param:"path=p,header=H,query=q,default=def"`
				F2 string `param:"header=H,query=q,path=p,default=def"`
				F3 string `param:"query=q,path=p,header=H,default=def"`
			}{
				F1: "pathVal",
				F2: "headerVal",
				F3: "queryVal",
			},
			expectErr: false,
		},
		"should handle whitespace in param tags correctly": {
			request: func() *http.Request {
				r := &http.Request{
					URL: &url.URL{
						RawQuery: "q=queryVal",
					},
					Header: http.Header{
						"H": []string{"headerVal"},
					},
				}
				r.SetPathValue("p", "pathVal")

				return r
			}(),
			output: &struct {
				Val1 string `param:" query = q "`
				Val2 string `param:" header = H "`
				Val3 string `param:" path = p "`
				Val4 string `param:" default = def "`
				Val5 string `param:" query = q , header = H "`
			}{},
			expected: &struct {
				Val1 string `param:" query = q "`
				Val2 string `param:" header = H "`
				Val3 string `param:" path = p "`
				Val4 string `param:" default = def "`
				Val5 string `param:" query = q , header = H "`
			}{
				Val1: "queryVal",
				Val2: "headerVal",
				Val3: "pathVal",
				Val4: "def",
				Val5: "queryVal",
			},
			expectErr: false,
		},
		"should skip unexported fields safely": {
			request: func() *http.Request {
				return &http.Request{
					URL: &url.URL{
						RawQuery: "visible=true",
					},
				}
			}(),
			output: &unexportedStruct{},
			expected: &unexportedStruct{
				visible: "", // Should be ignored and remain zero value
			},
			expectErr: false,
			cmpOptions: []cmp.Option{
				cmp.AllowUnexported(unexportedStruct{}),
			},
		},
		"should report correct parameter type when validation fails on a fallback source": {
			request: func() *http.Request {
				// Request with header 'H' (fallback) but no query 'q' (primary).
				// Value "abc" is too short (min=5).
				return &http.Request{
					URL: &url.URL{},
					Header: http.Header{
						"H": []string{"abc"},
					},
				}
			}(),
			output:      &fallbackStruct{},
			expectErr:   true,
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				// Should report 'q' (primary) as the parameter, but 'header' (actual source) as the type.
				{Parameter: "H", Detail: "should be min=5", Type: problem.ParameterTypeHeader},
			},
		},
		"should safely ignore malformed param tags": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "q=val",
				},
			},
			output: &struct {
				Val1 string `param:"query=q"`
				Val2 string `param:"malformed"`     // Missing =
				Val3 string `param:"k=v=extra"`     // Too many =
				Val4 string `param:"unknown=param"` // Unknown source
			}{},
			expected: &struct {
				Val1 string `param:"query=q"`
				Val2 string `param:"malformed"`
				Val3 string `param:"k=v=extra"`
				Val4 string `param:"unknown=param"`
			}{
				Val1: "val",
				Val2: "",
				Val3: "",
				Val4: "",
			},
			expectErr: false,
		},
		"should stop at default value even if subsequent sources are present": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "q=queryVal",
				},
				Header: http.Header{
					"H": []string{"headerVal"},
				},
			},
			output: &struct {
				// Default is placed before Header. Query is missing.
				// Should pick Default ("def") and ignore Header ("headerVal").
				Val string `param:"query=missing,default=def,header=H"`
			}{},
			expected: &struct {
				Val string `param:"query=missing,default=def,header=H"`
			}{
				Val: "def",
			},
			expectErr: false,
		},
		"should report correct parameter source for type conversion error on fallback": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "q=", // q is missing/empty, fallback to header H
				},
				Header: http.Header{
					"H": []string{"not-an-int"},
				},
			},
			output:      &fallbackTypeStruct{},
			expectErr:   true,
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "H", Detail: "must be a valid int", Type: problem.ParameterTypeHeader},
			},
		},
		"should collect multiple errors including conversion and validation": {
			request: &http.Request{
				URL: &url.URL{
					RawQuery: "ival=abc&sval=abc",
				},
				Header: http.Header{
					"H": []string{"not-an-int"},
				},
			},
			output:      &mixedErrorStruct{},
			expectErr:   true,
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "ival", Detail: "must be a valid int", Type: problem.ParameterTypeQuery},
				{Parameter: "H", Detail: "must be a valid int", Type: problem.ParameterTypeHeader},
				{Parameter: "sval", Detail: "should be min=5", Type: problem.ParameterTypeQuery},
			},
		}, "should skip validation when default value is used": {
			request: &http.Request{
				URL: &url.URL{}, // No query 'q'
			},
			output: &defaultValidationStruct{},
			expected: &defaultValidationStruct{
				Val: "abc",
			},
			expectErr: false,
		},
		"should report field name for validation error on field without param tag": {
			request:     &http.Request{URL: &url.URL{}},
			output:      &noParamTagStruct{},
			expectErr:   true,
			expectedErr: `400 Bad Parameters: The request parameters are invalid or malformed`,
			expectedParamErrors: []problem.Parameter{
				{Parameter: "Val", Detail: "is required", Type: ""},
			},
		},
		"should handle pure default parameter": {
			request:   &http.Request{URL: &url.URL{}},
			output:    &pureDefaultStruct{},
			expected:  &pureDefaultStruct{Val: 123},
			expectErr: false,
		},
		"should treat explicit empty query param as missing": {
			request: &http.Request{
				URL: &url.URL{RawQuery: "q="},
			},
			output:    &emptyValueStruct{},
			expected:  &emptyValueStruct{Val: ""},
			expectErr: false,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			err := httputil.BindValidParameters(testCase.request, testCase.output)

			if testCase.expectErr {
				if err == nil {
					t.Fatalf("want: error, got: nil")
				}

				if err.Error() != testCase.expectedErr {
					t.Fatalf("unexpected error message, got: %q, want: %q", err.Error(), testCase.expectedErr)
				}

				if len(testCase.expectedParamErrors) > 0 {
					var detailedErr *problem.DetailedError
					if !errors.As(err, &detailedErr) {
						t.Fatalf("expected error to be *problem.DetailedError, got %T", err)
					}

					v, ok := detailedErr.ExtensionMembers["violations"].([]problem.Parameter)
					if !ok {
						t.Fatalf("expected 'violations' extension member to be []problem.Parameter, got %T", detailedErr.ExtensionMembers["violations"])
					}

					if !cmp.Equal(v, testCase.expectedParamErrors) {
						t.Errorf("unexpected violations: %v", cmp.Diff(v, testCase.expectedParamErrors))
					}
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error, got: %v, want: nil", err)
			}

			if !cmp.Equal(testCase.output, testCase.expected, testCase.cmpOptions...) {
				t.Errorf("unexpected output, got: %+v, want: %+v", testCase.output, testCase.expected)
			}
		})
	}
}
