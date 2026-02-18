package httputil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/nickbryan/httputil/problem"
)

const (
	// tagParam is the struct tag for parameter binding.
	tagParam = "param"
	// sourceDefault identifies that the value came from the default declaration.
	sourceDefault = "default"
	// sourceQuery identifies that the value came from the URL query.
	sourceQuery = "query"
	// sourceHeader identifies that the value came from the HTTP headers.
	sourceHeader = "header"
	// sourcePath identifies that the value came from the URL path.
	sourcePath = "path"
	// tagPartSize is the expected number of parts when splitting a tag part by "=".
	tagPartSize = 2
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

// paramInfo holds metadata about a resolved parameter for error reporting.
type paramInfo struct {
	actualKey  string
	sourceType string
}

// resolvedParam represents the result of resolving a parameter value from a
// request or tag.
type resolvedParam struct {
	canonicalName string
	actualKey     string
	sourceType    string
	value         string
}

// reportingKey returns the key that should be used when reporting errors for the parameter.
func (p resolvedParam) reportingKey(fieldName string) string {
	k := p.actualKey
	if k == "" || k == sourceDefault {
		k = p.canonicalName
	}

	if k == "" {
		k = fieldName
	}

	return k
}

// paramTag represents the parsed content of a 'param' struct tag.
type paramTag struct {
	canonicalName string
	firstSource   string
	parts         []tagPart
}

// tagPart represents a single source/key pair within a 'param' tag.
type tagPart struct {
	source string
	key    string
}

// BindValidParameters extracts parameters from an *http.Request, populates the
// fields of the output struct, and validates the struct. The output parameter
// must be a pointer to a struct, and the struct fields can be annotated with
// struct tags to specify the source of the parameters. Supported struct tags
// and their meanings are:
//
//   - `param`: Specifies sources and options in "key=value" format, separated by commas.
//     Keys: query, header, path, default.
//     Order matters: first match wins.
//   - `validate`: Provides rules for the validator.
//
// Example:
//
//	 type Params struct {
//		  Sort	string  `param:"query=user_id" validate:"required"`
//		  AuthToken string  `param:"header=Authorization"`
//		  Page	  int	 `param:"query=page,default=1"`
//		  IsActive  bool	`param:"query=is_active,default=false"`
//		  ID		uuid.UUID `param:"path=id"`
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
// Validation is skipped for parameters that were populated from a `default`
// source. This allows developers to set default values that might strictly violate
// validation rules (e.g. zero values for required fields) without causing client-facing errors.
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

	paramTypes := make(map[string]paramInfo)

	var fieldsToSkip []string

	var query url.Values
	if r.URL != nil {
		query = r.URL.Query()
	}

	for i := range outputVal.NumField() {
		field := outputVal.Type().Field(i)
		if !field.IsExported() {
			continue
		}

		res := resolveParamValue(r, query, field)
		paramTypes[field.Name] = paramInfo{
			actualKey:  res.reportingKey(field.Name),
			sourceType: res.sourceType,
		}

		if res.actualKey == sourceDefault {
			fieldsToSkip = append(fieldsToSkip, field.Name)
		}

		if res.value != "" {
			paramErrors, err = setFieldAndHandleError(outputVal.Field(i), res, paramErrors)
			if err != nil {
				return err
			}
		}
	}

	paramErrors, err = validateStruct(r.Context(), output, paramTypes, paramErrors, fieldsToSkip)
	if err != nil {
		return err
	}

	if len(paramErrors) > 0 {
		return problem.BadParameters(r, paramErrors...)
	}

	return nil
}

// setFieldAndHandleError attempts to set a struct field's value and handles any
// conversion errors that occur by appending them to the provided error slice.
func setFieldAndHandleError(
	fieldVal reflect.Value,
	res resolvedParam,
	paramErrors []problem.Parameter,
) ([]problem.Parameter, error) {
	if err := setFieldValue(fieldVal, res.actualKey, res.value, res.sourceType); err != nil {
		if paramConversionError, ok := errors.AsType[*ParamConversionError](err); res.actualKey != sourceDefault && ok {
			paramErrors = append(paramErrors, problem.Parameter{
				Parameter: paramConversionError.ParamName,
				Detail:    "must be a valid " + paramConversionError.TargetType,
				Type:      paramConversionError.ParameterType,
			})

			return paramErrors, nil
		}

		// If the paramName == "default" then the error was on the developer setting the
		// default value, so we don't want to show that in the response, treat it as an
		// error instead of a problem with the request.
		return nil, fmt.Errorf("setting field value: %w", err)
	}

	return paramErrors, nil
}

// validateStruct performs validation on the struct and processes any errors.
func validateStruct(
	ctx context.Context,
	output any,
	paramTypes map[string]paramInfo,
	paramErrors []problem.Parameter,
	fieldsToSkip []string,
) ([]problem.Parameter, error) {
	if err := validate.StructExceptCtx(ctx, output, fieldsToSkip...); err != nil {
		if errs, ok := errors.AsType[validator.ValidationErrors](err); ok {
			paramErrors = append(paramErrors, processValidationErrors(errs, paramTypes)...)
		} else {
			return nil, fmt.Errorf("validating struct: %w", err)
		}
	}

	return paramErrors, nil
}

// processValidationErrors converts validator errors to problem parameters.
func processValidationErrors(errs validator.ValidationErrors, paramTypes map[string]paramInfo) []problem.Parameter {
	validationErrors := make([]problem.Parameter, 0, len(errs))

	for _, err := range errs {
		info := paramTypes[err.StructField()]

		validationErrors = append(validationErrors, problem.Parameter{
			Parameter: info.actualKey,
			Detail:    describeValidationError(err),
			Type:      problem.ParameterType(info.sourceType),
		})
	}

	return validationErrors
}

// validateOutputType ensures the `output` is a pointer to a struct and returns
// its dereferenced value or an error.
func validateOutputType(output any) (reflect.Value, error) {
	outputVal := reflect.ValueOf(output)
	if outputVal.Kind() != reflect.Pointer || outputVal.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, &InvalidOutputTypeError{ProvidedType: output}
	}

	return outputVal.Elem(), nil
}

// resolveParamValue extracts a named parameter's value from an HTTP request
// using struct field tags (query, header, path, default).
func resolveParamValue(r *http.Request, query url.Values, field reflect.StructField) resolvedParam {
	tag := parseParamTag(field.Tag.Get(tagParam))
	if tag == nil {
		return resolvedParam{
			canonicalName: "",
			actualKey:     "",
			sourceType:    "",
			value:         "",
		}
	}

	for _, part := range tag.parts {
		if part.source == sourceDefault {
			return resolvedParam{
				canonicalName: tag.canonicalName,
				actualKey:     sourceDefault,
				sourceType:    tag.firstSource,
				value:         part.key,
			}
		}

		if value := getSourceValue(r, query, part.source, part.key); value != "" {
			return resolvedParam{
				canonicalName: tag.canonicalName,
				actualKey:     part.key,
				sourceType:    part.source,
				value:         value,
			}
		}
	}

	return resolvedParam{
		canonicalName: tag.canonicalName,
		actualKey:     tag.canonicalName,
		sourceType:    tag.firstSource,
		value:         "",
	}
}

// parseParamTag parses a 'param' struct tag into a paramTag struct.
func parseParamTag(tagStr string) *paramTag {
	if tagStr == "" {
		return nil
	}

	res := &paramTag{
		canonicalName: "",
		firstSource:   "",
		parts:         nil,
	}

	for part := range strings.SplitSeq(tagStr, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", tagPartSize)
		if len(kv) != tagPartSize {
			continue
		}

		source, key := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		res.parts = append(res.parts, tagPart{source: source, key: key})

		if res.canonicalName == "" && source != sourceDefault {
			res.canonicalName, res.firstSource = key, source
		}
	}

	if res.canonicalName == "" && len(res.parts) > 0 {
		res.canonicalName = sourceDefault
	}

	return res
}

// getSourceValue retrieves the value for a given source and key from an HTTP
// request.
func getSourceValue(r *http.Request, query url.Values, source, key string) string {
	switch source {
	case sourceQuery:
		return query.Get(key)
	case sourceHeader:
		return r.Header.Get(key)
	case sourcePath:
		return r.PathValue(key)
	default:
		return ""
	}
}

// setFieldValue assigns a parameter value to a struct field, converting it to
// the appropriate type or returning an error.
func setFieldValue(fieldVal reflect.Value, paramName, paramValue, paramType string) error {
	if _, ok := fieldVal.Interface().(uuid.UUID); ok {
		return setUUIDField(fieldVal, paramName, paramValue, paramType)
	}

	switch fieldVal.Kind() {
	case reflect.String:
		return setStringField(fieldVal, paramValue)
	case reflect.Int:
		return setIntField(fieldVal, paramName, paramValue, paramType)
	case reflect.Bool:
		return setBoolField(fieldVal, paramName, paramValue, paramType)
	case reflect.Float64:
		return setFloatField(fieldVal, paramName, paramValue, paramType)
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
