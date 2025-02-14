package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
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

type ParamType interface {
	bool | float64 | int | string | time.Time | uuid.UUID
}

type ParamBuilder[T ParamType] struct {
	orValue     T
	timeLayout  string
	inputRules  string
	outputRules string
	value       string
}

func HeaderParam[T ParamType](r *http.Request, key string) *ParamBuilder[T] {
	return newParamBuilder[T](r.Header.Get(key))
}

func PathParam[T ParamType](r *http.Request, key string) *ParamBuilder[T] {
	return newParamBuilder[T](r.PathValue(key))
}

func QueryParam[T ParamType](r *http.Request, key string) *ParamBuilder[T] {
	return newParamBuilder[T](r.URL.Query().Get(key))
}

func newParamBuilder[T ParamType](value string) *ParamBuilder[T] {
	var zero T

	return &ParamBuilder[T]{
		orValue:     zero,
		timeLayout:  time.RFC3339,
		inputRules:  "",
		outputRules: "",
		value:       value,
	}
}

// Or sets the default value if the parameter is missing or invalid.
func (b *ParamBuilder[T]) Or(defaultValue T) *ParamBuilder[T] {
	b.orValue = defaultValue
	return b
}

// InputRules adds input validation rules using the go-playground/validator syntax.
func (b *ParamBuilder[T]) InputRules(rules string) *ParamBuilder[T] {
	b.inputRules = rules
	return b
}

// OutputRules adds output validation rules using the go-playground/validator syntax.
func (b *ParamBuilder[T]) OutputRules(rules string) *ParamBuilder[T] {
	b.outputRules = rules
	return b
}

// TimeLayout allows setting custom time layout. The default is time.RFC3339.
func (b *ParamBuilder[T]) TimeLayout(layout string) *ParamBuilder[T] {
	b.timeLayout = layout
	return b
}

// Resolve retrieves the parsed value.
func (b *ParamBuilder[T]) Resolve() (T, error) {
	if b.inputRules != "" {
		if err := validate.Var(b.value, b.inputRules); err != nil {
			return b.orValue, err
		}
	}

	if b.value == "" {
		return b.orValue, nil
	}

	var result T

	switch any(result).(type) {
	case bool:
		var val bool

		val, err := strconv.ParseBool(b.value)
		if err != nil {
			return b.orValue, fmt.Errorf("parsing bool: %w", err)
		}

		result = any(val).(T)
	case float64:
		var val float64

		val, err := strconv.ParseFloat(b.value, 64)
		if err != nil {
			return b.orValue, fmt.Errorf("parsing float: %w", err)
		}

		result = any(val).(T)
	case int:
		var val int

		val, err := strconv.Atoi(b.value)
		if err != nil {
			return b.orValue, fmt.Errorf("parsing int: %w", err)
		}

		result = any(val).(T)
	case string:
		result = any(b.value).(T)
	case time.Time:
		var val time.Time

		val, err := time.Parse(b.timeLayout, b.value)
		if err != nil {
			return b.orValue, fmt.Errorf("parsing time: %w", err)
		}

		result = any(val).(T)
	case uuid.UUID:
		var val uuid.UUID

		val, err := uuid.Parse(b.value)
		if err != nil {
			return b.orValue, fmt.Errorf("parsing uuid: %w", err)
		}

		result = any(val).(T)
	default:
		return b.orValue, errors.New("unsupported type")
	}

	if b.outputRules != "" {
		if err := validate.Var(result, b.outputRules); err != nil {
			return b.orValue, err
		}
	}

	return result, nil
}

func Params(r *http.Request, output any) error {
	outputVal := reflect.ValueOf(output).Elem() // Get the addressable value of the output struct
	if outputVal.Kind() != reflect.Struct {
		return fmt.Errorf("output must be a pointer to a struct, got %T", output)
	}

	for i := range outputVal.NumField() {
		field := outputVal.Type().Field(i)

		queryTag := field.Tag.Get("query")
		headerTag := field.Tag.Get("header")
		pathTag := field.Tag.Get("path")

		if queryTag == "" && headerTag == "" && pathTag == "" {
			continue
		}

		var paramName, paramValue string

		if queryTag != "" {
			paramName = queryTag
			paramValue = r.URL.Query().Get(paramName)
		} else if headerTag != "" {
			paramName = headerTag
			paramValue = r.Header.Get(paramName)
		} else if pathTag != "" {
			paramName = pathTag
			paramValue = r.PathValue(paramName)
		}

		if paramValue == "" {
			continue
		}

		fieldVal := outputVal.Field(i) // Get the value of the current field

		switch fieldVal.Interface().(type) {
		case string:
			fieldVal.SetString(paramValue)
		case int:
			v, err := strconv.Atoi(paramValue)
			if err != nil {
				return fmt.Errorf("failed to convert %s to int: %w", paramName, err)
			}

			fieldVal.SetInt(int64(v))
		case bool:
			v, err := strconv.ParseBool(paramValue)
			if err != nil {
				return fmt.Errorf("failed to convert %s to bool: %w", paramName, err)
			}

			fieldVal.SetBool(v)
		case float64:
			v, err := strconv.ParseFloat(paramValue, 64)
			if err != nil {
				return fmt.Errorf("failed to convert %s to float64: %w", paramName, err)
			}

			fieldVal.SetFloat(v)
		case time.Time:
			v, err := time.Parse(time.RFC3339, paramValue)
			if err != nil {
				return fmt.Errorf("failed to convert %s to time.Time: %w", paramName, err)
			}

			fieldVal.Set(reflect.ValueOf(v))
		case uuid.UUID:
			v, err := uuid.Parse(paramValue)
			if err != nil {
				return fmt.Errorf("failed to convert %s to uuid.UUID: %w", paramName, err)
			}

			fieldVal.Set(reflect.ValueOf(v))
		default:
			return fmt.Errorf("unsupported field type: %T", fieldVal.Interface())
		}
	}

	return nil
}
