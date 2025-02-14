package httputil

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"github.com/google/uuid"
)

func decodeParams(r *http.Request, output any) error {
	outputVal := reflect.ValueOf(output).Elem()
	if outputVal.Kind() != reflect.Struct {
		return fmt.Errorf("output must be a pointer to a struct, got %T", output)
	}

	for i := range outputVal.NumField() {
		field := outputVal.Type().Field(i)

		queryTag := field.Tag.Get("query")
		headerTag := field.Tag.Get("header")
		pathTag := field.Tag.Get("path")
		defaultTag := field.Tag.Get("default")

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

		if paramValue == "" && defaultTag != "" {
			paramValue = defaultTag
		}

		if paramValue == "" {
			continue
		}

		fieldVal := outputVal.Field(i)

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
