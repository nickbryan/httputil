package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
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
	ParamName  string
	TargetType string
	Err        error
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

func UnmarshalParamsAndValidate(r *http.Request, output any, validator *validator.Validate) error {
	if validator == nil {
		return errors.New("validator is nil")
	}

	if err := UnmarshalParams(r, output); err != nil {
		return err
	}

	if err := validator.Struct(output); err != nil {
		return err
	}

	return nil
}

// UnmarshalParams extracts parameters from an *http.Request and populates the fields of the output struct.
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
//	 if err := UnmarshalParams(r, &params); err != nil {
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
func UnmarshalParams(r *http.Request, output any) error {
	outputVal, err := validateOutputType(output)
	if err != nil {
		return fmt.Errorf("validating output type: %w", err)
	}

	for i := range outputVal.NumField() {
		field := outputVal.Type().Field(i)

		paramName, paramValue := resolveParamValue(r, field)
		if paramValue == "" {
			continue
		}

		if err := setFieldValue(outputVal.Field(i), paramName, paramValue); err != nil {
			return fmt.Errorf("setting field value: %w", err)
		}
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

func resolveParamValue(r *http.Request, field reflect.StructField) (string, string) {
	const (
		tagQuery   = "query"
		tagHeader  = "header"
		tagPath    = "path"
		tagDefault = "default"
	)

	var paramName, paramValue string

	queryTag := field.Tag.Get(tagQuery)
	headerTag := field.Tag.Get(tagHeader)
	pathTag := field.Tag.Get(tagPath)
	defaultTag := field.Tag.Get(tagDefault)

	if queryTag != "" {
		paramName, paramValue = queryTag, r.URL.Query().Get(queryTag)
	}

	if paramValue == "" && headerTag != "" {
		paramName, paramValue = headerTag, r.Header.Get(headerTag)
	}

	if paramValue == "" && pathTag != "" {
		paramName, paramValue = pathTag, r.PathValue(pathTag)
	}

	if paramValue == "" && defaultTag != "" {
		paramName, paramValue = tagDefault, defaultTag
	}

	return paramName, paramValue
}

func setFieldValue(fieldVal reflect.Value, paramName, paramValue string) error {
	switch fieldVal.Interface().(type) {
	case string:
		return setStringField(fieldVal, paramName, paramValue)
	case int:
		return setIntField(fieldVal, paramName, paramValue)
	case bool:
		return setBoolField(fieldVal, paramName, paramValue)
	case float64:
		return setFloatField(fieldVal, paramName, paramValue)
	case uuid.UUID:
		return setUUIDField(fieldVal, paramName, paramValue)
	default:
		return &UnsupportedFieldTypeError{FieldType: fieldVal.Interface()}
	}
}

func setStringField(fieldVal reflect.Value, _, paramValue string) error {
	fieldVal.SetString(paramValue)
	return nil
}

func setIntField(fieldVal reflect.Value, paramName, paramValue string) error {
	v, err := strconv.Atoi(paramValue)
	if err != nil {
		return &ParamConversionError{
			ParamName:  paramName,
			TargetType: "int",
			Err:        err,
		}
	}

	fieldVal.SetInt(int64(v))

	return nil
}

func setBoolField(fieldVal reflect.Value, paramName, paramValue string) error {
	v, err := strconv.ParseBool(paramValue)
	if err != nil {
		return &ParamConversionError{
			ParamName:  paramName,
			TargetType: "bool",
			Err:        err,
		}
	}

	fieldVal.SetBool(v)

	return nil
}

func setFloatField(fieldVal reflect.Value, paramName, paramValue string) error {
	v, err := strconv.ParseFloat(paramValue, 64)
	if err != nil {
		return &ParamConversionError{
			ParamName:  paramName,
			TargetType: "float64",
			Err:        err,
		}
	}

	fieldVal.SetFloat(v)

	return nil
}

func setUUIDField(fieldVal reflect.Value, paramName, paramValue string) error {
	v, err := uuid.Parse(paramValue)
	if err != nil {
		return &ParamConversionError{
			ParamName:  paramName,
			TargetType: "uuid.UUID",
			Err:        err,
		}
	}

	fieldVal.Set(reflect.ValueOf(v))

	return nil
}
