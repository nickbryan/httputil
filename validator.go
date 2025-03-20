package httputil

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// As pert the validator.New docs:
//
// InputRules is designed to be thread-safe and used as a singleton instance.
// It caches information about your struct and validations,
// in essence only parsing your validation tags once per struct type.
// Using multiple instances neglects the benefit of caching.
//
// Doing this allows for a much cleaner API too.
//
//nolint:gochecknoglobals // See the comment above.
var validate *validator.Validate

//nolint:gochecknoinits // Required to create our singleton instance of the validator.
func init() {
	validate = defaultValidator()
}

// defaultValidator returns a new validator.Validate that is configured for JSON tags.
func defaultValidator() *validator.Validate {
	vld := validator.New(validator.WithRequiredStructEnabled())

	vld.RegisterTagNameFunc(func(f reflect.StructField) string {
		const jsonTags = 2 // `json:"field,omitempty"`

		name := strings.SplitN(f.Tag.Get("json"), ",", jsonTags)[0]
		if name == "-" {
			return ""
		}

		if name == "" {
			name = f.Tag.Get("query")
		}

		if name == "" {
			name = f.Tag.Get("path")
		}

		if name == "" {
			name = f.Tag.Get("header")
		}

		return name
	})

	return vld
}

// describeValidationError generates a human-readable error message based on the violated validation tag of a field.
func describeValidationError(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return err.Field() + " is required"
	case "email":
		return err.Field() + " should be a valid email"
	case "e164":
		return err.Field() + " should be a valid international phone number (e.g. +33 6 06 06 06 06)"
	default:
		if strings.Contains(err.Tag(), "uuid") {
			return err.Field() + " should be a valid " + strings.ToUpper(err.Tag())
		}

		resp := fmt.Sprintf("%s should be %s", err.Field(), err.Tag())
		if err.Param() != "" {
			resp += "=" + err.Param()
		}

		return resp
	}
}
