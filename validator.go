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
var validate *validator.Validate

func init() {
	validate = defaultValidator()
}

// defaultValidator returns a new validator.Validate that is configured for JSON tags.
func defaultValidator() *validator.Validate {
	vld := validator.New(validator.WithRequiredStructEnabled())

	vld.RegisterTagNameFunc(func(f reflect.StructField) string {
		const tags = 2
		name := strings.SplitN(f.Tag.Get("json"), ",", tags)[0]

		if name == "-" {
			return ""
		}

		if name == "" {
			name = strings.SplitN(f.Tag.Get("query"), ",", tags)[0]
		}

		if name == "" {
			name = strings.SplitN(f.Tag.Get("path"), ",", tags)[0]
		}

		if name == "" {
			name = strings.SplitN(f.Tag.Get("header"), ",", tags)[0]
		}

		return name
	})

	return vld
}

func explainValidationError(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return err.Field() + " is required"
	case "email":
		return err.Field() + " should be a valid email"
	case "uuid":
		return err.Field() + " should be a valid UUID"
	case "e164":
		return err.Field() + " should be a valid international phone number (e.g. +33 6 06 06 06 06)"
	default:
		resp := fmt.Sprintf("%s should be %s", err.Field(), err.Tag())
		if err.Param() != "" {
			resp += "=" + err.Param()
		}

		return resp
	}
}
