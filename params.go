package httputil

import (
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
	tagQuery   = "query"
	tagHeader  = "header"
	tagPath    = "path"
	tagDefault = "default"
)

// InvalidOutputTypeError is a custom error type for invalid output types.
type InvalidOutputTypeError struct {
	ProvidedType any
}

func (e *InvalidOutputTypeError) Error() string {
	return fmt.Sprintf("output must be a pointer to a struct, got %T", e.ProvidedType)
}

// ParamConversionError represents an error that occurs during parameter conversion.
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

// BindValidParameters extracts parameters from an *http.Request and populates the fields of the output struct.
// The output parameter must be a pointer to a struct, and the struct fields can be annotated with struct tags
// to specify the source of the parameters. Supported struct tags and their meanings are:
//
// - `query`: Specifies a query parameter to extract from the URL.
// - `header`: Specifies an HTTP header to extract from the request.
// - `path`: Specifies a path parameter to extract. Requires an implementation of r.PathValue().
// - `default`: Provides a default value for the parameter if it's not found in the request.
//
// Example:
//
//	 type Params struct {
//		  Sort	string  `query:"user_id"`
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
// Returns an error if:
// - `output` is not a pointer to a struct.
// - A parameter value cannot be converted to the target field type.
// - A field type in the struct is unsupported.
func BindValidParameters(r *http.Request, v *validator.Validate, output any) error {
	outputVal, err := validateOutputType(output)
	if err != nil {
		return fmt.Errorf("validating output type: %w", err)
	}

	var paramErrors []problem.Parameter

	for i := range outputVal.NumField() {
		field := outputVal.Type().Field(i)

		paramName, paramValue, paramType := resolveParamValue(r, field)
		if paramValue == "" {
			continue
		}

		// TODO: update documentation for function.
		// TODO: thoroughly test validation and v being nil.
		// TODO: try this out in a guard or something to see how dealing with errors feels.
		// TODO: update params_test.go to cover this properly.
		// TODO: revisit handler_json_test.go to see what needs to be complete for params handling.
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

	if v != nil {
		if err := v.StructCtx(r.Context(), output); err != nil {
			var errs validator.ValidationErrors
			if errors.As(err, &errs) {
				for _, err := range errs {
					paramErrors = append(paramErrors, problem.Parameter{
						Parameter: strings.Join(strings.Split(err.Namespace(), ".")[1:], "."),
						Detail:    explainValidationError(err),
						Type:      problem.ParameterType(err.Tag()),
					})
				}
			} else {
				return fmt.Errorf("validating struct: %w", err)
			}
		}
	}

	if len(paramErrors) > 0 {
		return problem.BadParameters(r, paramErrors...)
	}

	return nil
}

func validateOutputType(output any) (reflect.Value, error) {
	outputVal := reflect.ValueOf(output)
	if outputVal.Kind() != reflect.Ptr || outputVal.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, &InvalidOutputTypeError{ProvidedType: output}
	}

	return outputVal.Elem(), nil
}

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

func setStringField(fieldVal reflect.Value, paramValue string) error {
	fieldVal.SetString(paramValue)
	return nil
}

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
