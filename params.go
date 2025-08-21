package httputil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/nickbryan/httputil/problem"
)

const (
	// tagQuery is the struct tag for query parameters.
	tagQuery   = "query"
	// tagHeader is the struct tag for header parameters.
	tagHeader  = "header"
	// tagPath is the struct tag for path parameters.
	tagPath    = "path"
	// tagDefault is the struct tag for default values.
	tagDefault = "default"
)

// InvalidOutputTypeError is a custom error type for invalid output types.
type InvalidOutputTypeError struct {
	ProvidedType any
}

// Error returns the error message describing the provided invalid output type.
func (e *InvalidOutputTypeError) Error() string {
	return fmt.Sprintf("output must be a pointer to a struct, got %T", e.ProvidedType)
}

// ParamConversionError represents an error that occurs during parameter
// conversion.
type ParamConversionError struct {
	ParameterType problem.ParameterType
	ParamName     string
	TargetType    string
	Err           error
}

// Error satisfies the error interface for ParamConversionError.
func (e *ParamConversionError) Error() string {
	return fmt.Sprintf("failed to convert parameter %q to %s: %v", e.ParamName, e.TargetType, e.Err)
}

// Unwrap allows ParamConversionError to be used with errors.Is and errors.As.
func (e *ParamConversionError) Unwrap() error {
	return e.Err
}

// UnsupportedFieldTypeError represents an error for unsupported field types.
type UnsupportedFieldTypeError struct {
	FieldType any
}

// Error satisfies the error interface for UnsupportedFieldTypeError.
func (e *UnsupportedFieldTypeError) Error() string {
	return fmt.Sprintf("unsupported field type: %T", e.FieldType)
}

// BindValidParameters extracts parameters from an *http.Request, populates the
// fields of the output struct, and validates the struct. The output parameter
// must be a pointer to a struct, and the struct fields can be annotated with
// struct tags to specify the source of the parameters. Supported struct tags
// and their meanings are:
//
// - `query`: Specifies a query parameter to extract from the URL.
// - `header`: Specifies an HTTP header to extract from the request.
// - `path`: Specifies a path parameter to extract. Requires an implementation of r.PathValue().
// - `default`: Provides a default value for the parameter if it's not found in the request.
// - `validate`: Provides rules for the validator.
//
// Example:
//
//	 type Params struct {
//		  Sort	string  `query:"user_id" validate:"required"`
//		  AuthToken string  `header:"Authorization"`
//		  Page	  int	 `query:"page" default:"1"`
//		  IsActive  bool	`query:"is_active" default:"false"`
//		  ID		uuid.UUID `path:"id"`
//	 }
//	 var params Params
//	 if err := BindValidParameters(r, &params); err != nil {
//		  return fmt.Errorf("failed to unmarshal params: %v", err)
//	 }
//
// If a field's type is unsupported, or if a value cannot be converted to the target type,
// a descriptive error is returned. Supported field types include:
//
// - string
// - int
// - bool
// - float64
// - uuid.UUID
//
// Returns problem.BadParameters if:
// - A value cannot be converted to the target field type.
// - Validation fails.
//
// Returns an error if:
// - `output` is not a pointer to a struct.
// - A default value cannot be converted to the target field type.
// - A field type in the struct is unsupported.
func BindValidParameters(r *http.Request, output any) error {
	outputVal, err := validateOutputType(output)
	if err != nil {
		return fmt.Errorf("validating output type: %w", err)
	}

	var paramErrors []problem.Parameter

	paramTypes := make(map[string]string)

	for i := range outputVal.NumField() {
		field := outputVal.Type().Field(i)

		paramName, paramValue, paramType := resolveParamValue(r, field)
		paramTypes[paramName] = paramType

		if paramValue == "" {
			continue
		}

		if err := setFieldValue(outputVal.Field(i), paramName, paramValue, paramType); err != nil {
			var paramConversionError *ParamConversionError
			if paramName != tagDefault && errors.As(err, &paramConversionError) {
				paramErrors = append(paramErrors, problem.Parameter{
					Parameter: paramConversionError.ParamName,
					Detail:    paramConversionError.Err.Error(),
					Type:      paramConversionError.ParameterType,
				})

				continue
			}

			// If the paramName == "default" then the error was on the developer setting the
			// default value so we don't want to show that in the response, treat it as an
			// error instead of a problem with the request.
			return fmt.Errorf("setting field value: %w", err)
		}
	}

	paramErrors, err = validateStruct(r.Context(), output, paramTypes, paramErrors)
	if err != nil {
		return err
	}

	if len(paramErrors) > 0 {
		return problem.BadParameters(r, paramErrors...)
	}

	return nil
}

// validateStruct performs validation on the struct and processes any errors.
func validateStruct(ctx context.Context, output any, paramTypes map[string]string, paramErrors []problem.Parameter) ([]problem.Parameter, error) {
	if err := validate.StructCtx(ctx, output); err != nil {
		var errs validator.ValidationErrors

		if errors.As(err, &errs) {
			paramErrors = append(paramErrors, processValidationErrors(errs, paramTypes)...)
		} else {
			return nil, fmt.Errorf("validating struct: %w", err)
		}
	}

	return paramErrors, nil
}

// processValidationErrors converts validator errors to problem parameters.
func processValidationErrors(errs validator.ValidationErrors, paramTypes map[string]string) []problem.Parameter {
	validationErrors := make([]problem.Parameter, 0, len(errs))

	for _, err := range errs {
		fieldName := strings.Join(strings.Split(err.Namespace(), ".")[1:], ".")

		validationErrors = append(validationErrors, problem.Parameter{
			Parameter: fieldName,
			Detail:    describeValidationError(err),
			Type:      problem.ParameterType(paramTypes[fieldName]),
		})
	}

	return validationErrors
}

// validateOutputType ensures the `output` is a pointer to a struct and returns
// its dereferenced value or an error.
func validateOutputType(output any) (reflect.Value, error) {
	outputVal := reflect.ValueOf(output)
	if outputVal.Kind() != reflect.Ptr || outputVal.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, &InvalidOutputTypeError{ProvidedType: output}
	}

	return outputVal.Elem(), nil
}

// resolveParamValue extracts a named parameter's value from an HTTP request
// using struct field tags (query, header, path, default). Returns the parameter
// name, value, and source type; returns empty strings if no value is found.
func resolveParamValue(r *http.Request, field reflect.StructField) (string, string, string) {
	var paramName, paramValue, paramType string

	if key := field.Tag.Get(tagQuery); key != "" {
		paramName, paramValue, paramType = key, r.URL.Query().Get(key), tagQuery
	}

	if paramValue != "" {
		return paramName, paramValue, paramType
	}

	if key := field.Tag.Get(tagHeader); key != "" {
		paramName, paramValue, paramType = key, r.Header.Get(key), tagHeader
	}

	if paramValue != "" {
		return paramName, paramValue, paramType
	}

	if key := field.Tag.Get(tagPath); key != "" {
		paramName, paramValue, paramType = key, r.PathValue(key), tagPath
	}

	if paramValue != "" {
		return paramName, paramValue, paramType
	}

	if value := field.Tag.Get(tagDefault); value != "" {
		paramName, paramValue = tagDefault, value
	}

	return paramName, paramValue, paramType
}

// setFieldValue assigns a parameter value to a struct field, converting it to
// the appropriate type or returning an error.
func setFieldValue(fieldVal reflect.Value, paramName, paramValue, paramType string) error {
	switch fieldVal.Interface().(type) {
	case string:
		return setStringField(fieldVal, paramValue)
	case int:
		return setIntField(fieldVal, paramName, paramValue, paramType)
	case bool:
		return setBoolField(fieldVal, paramName, paramValue, paramType)
	case float64:
		return setFloatField(fieldVal, paramName, paramValue, paramType)
	case uuid.UUID:
		return setUUIDField(fieldVal, paramName, paramValue, paramType)
	default:
		return &UnsupportedFieldTypeError{FieldType: fieldVal.Interface()}
	}
}

// setStringField assigns a string value to a reflect.Value field. Returns an
// error if the operation fails.
func setStringField(fieldVal reflect.Value, paramValue string) error {
	fieldVal.SetString(paramValue)
	return nil
}

// setIntField assigns an integer value to a reflect.Value field after
// converting it from a string. Returns an error if the conversion fails.
func setIntField(fieldVal reflect.Value, paramName, paramValue, paramType string) error {
	v, err := strconv.Atoi(paramValue)
	if err != nil {
		return &ParamConversionError{
			ParameterType: problem.ParameterType(paramType),
			ParamName:     paramName,
			TargetType:    "int",
			Err:           err,
		}
	}

	fieldVal.SetInt(int64(v))

	return nil
}

// setBoolField sets a boolean value to a reflect.Value field after converting
// it from a string. Returns an error on failure.
func setBoolField(fieldVal reflect.Value, paramName, paramValue, paramType string) error {
	v, err := strconv.ParseBool(paramValue)
	if err != nil {
		return &ParamConversionError{
			ParameterType: problem.ParameterType(paramType),
			ParamName:     paramName,
			TargetType:    "bool",
			Err:           err,
		}
	}

	fieldVal.SetBool(v)

	return nil
}

// setFloatField assigns a float64 value to a reflect.Value field after
// converting from a string. Returns an error on failure.
func setFloatField(fieldVal reflect.Value, paramName, paramValue, paramType string) error {
	v, err := strconv.ParseFloat(paramValue, 64)
	if err != nil {
		return &ParamConversionError{
			ParameterType: problem.ParameterType(paramType),
			ParamName:     paramName,
			TargetType:    "float64",
			Err:           err,
		}
	}

	fieldVal.SetFloat(v)

	return nil
}

// setUUIDField parses a UUID string and sets it to the provided reflect.Value
// field. Returns an error on parsing failure.
func setUUIDField(fieldVal reflect.Value, paramName, paramValue, paramType string) error {
	v, err := uuid.Parse(paramValue)
	if err != nil {
		return &ParamConversionError{
			ParameterType: problem.ParameterType(paramType),
			ParamName:     paramName,
			TargetType:    "uuid.UUID",
			Err:           err,
		}
	}

	fieldVal.Set(reflect.ValueOf(v))

	return nil
}
