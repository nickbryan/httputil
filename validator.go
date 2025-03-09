package httputil

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate is a singleton instance of *validator.Validate used for struct and field validation to take advantage of
// caching.
var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())

	validate.RegisterTagNameFunc(func(f reflect.StructField) string {
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
}

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
