package httputil

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/form/v4"
	"github.com/go-playground/validator/v10"

	"github.com/nickbryan/httputil/problem"
)

const (
	invalidValueMessage = "invalid value"
)

// validate is a singleton instance of the validator.Validate type used for
// validating request data and parameters.
//
// As per the validator.New docs:
//
// InputRules is designed to be thread-safe and used as a singleton instance. It
// caches information about your struct and validations, in essence only parsing
// your validation tags once per struct type. Using multiple instances neglects
// the benefit of caching.
//
// Doing this allows for a much cleaner API too.
//
//nolint:gochecknoglobals // See the comment above.
var validate *validator.Validate

//nolint:gochecknoinits // Required to create our singleton instance of the validator.
func init() {
	validate = defaultValidator()
}

// defaultValidator returns a new validator.Validate that is configured for JSON
// tags.
func defaultValidator() *validator.Validate {
	vld := validator.New(validator.WithRequiredStructEnabled())

	vld.RegisterTagNameFunc(func(f reflect.StructField) string {
		const maxParts = 2 // e.g. `json:"field,omitempty"`

		// Check json tag first, then form tag (HTML form handlers).
		for _, tag := range []string{"json", "form"} {
			name := strings.SplitN(f.Tag.Get(tag), ",", maxParts)[0]
			if name == "-" {
				return ""
			}

			if name != "" {
				return name
			}
		}

		return ""
	})

	return vld
}

// MessageFunc generates a user-facing error message for a validation failure.
// The tag is the validation rule that failed (e.g. "required", "min", "email")
// and param is its argument (e.g. "5" for min=5, empty for required).
//
// Use [WithHandlerMessages] to provide a custom MessageFunc — for example, to
// support i18n:
//
//	httputil.NewHandler(action, httputil.WithHandlerMessages(
//	    func(tag, param string) string {
//	        return i18n.T(tag, param)
//	    },
//	))
type MessageFunc func(tag, param string) string

// describeValidationError generates a human-readable error message based on the
// violated validation tag of a field.
func describeValidationError(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return "is required"
	case "email":
		return "should be a valid email"
	case "e164":
		return "should be a valid international phone number (e.g. +33 6 06 06 06 06)"
	default:
		if strings.Contains(err.Tag(), "uuid") {
			return "should be a valid " + strings.ToUpper(err.Tag())
		}

		resp := "should be " + err.Tag()
		if err.Param() != "" {
			resp += "=" + err.Param()
		}

		return resp
	}
}

// translateDataError translates request body decoding and validation errors
// into a field-to-message map. When fn is non-nil it is used for validation
// errors; otherwise [describeValidationError] provides the default messages.
// Decode errors (type conversion failures) always use a generic message.
//
// Field keys use dot-separated paths for nested structs (e.g. "address.city").
func translateDataError(err error, fn MessageFunc) map[string]string {
	result := make(map[string]string)

	if errs, ok := errors.AsType[validator.ValidationErrors](err); ok {
		for _, e := range errs {
			parts := strings.Split(e.Namespace(), ".")
			field := strings.Join(parts[1:], ".")

			if fn != nil {
				result[field] = fn(e.Tag(), e.Param())
			} else {
				result[field] = describeValidationError(e)
			}
		}

		return result
	}

	if decodeErrs, ok := errors.AsType[form.DecodeErrors](err); ok {
		for field, fieldErr := range decodeErrs {
			_ = fieldErr // Use generic message to avoid exposing internals.
			result[field] = invalidValueMessage
		}

		return result
	}

	return result
}

// translateParamsError translates parameter binding and validation errors into
// a parameter-to-message map.
func translateParamsError(err error) map[string]string {
	result := make(map[string]string)

	if details, ok := errors.AsType[*problem.DetailedError](err); ok {
		if violations, vOK := details.ExtensionMembers["violations"].([]problem.Parameter); vOK {
			for _, v := range violations {
				result[v.Parameter] = v.Detail
			}
		}
	}

	return result
}
