package httputil

import (
	"maps"
)

// BindErrors aggregates validation and binding errors from request processing.
// It provides helper methods for extracting translated, user-friendly error
// messages, which is particularly useful when rendering HTML templates with
// inline validation errors.
//
// Field keys use dot-separated paths matching struct tag names (e.g.
// "address.city" for a nested City field with a json/form tag of "city").
//
// Note: [BindErrors.HasAny] reports whether any error occurred during binding,
// but [BindErrors.Get] and [BindErrors.All] only contain entries for error types
// that could be mapped to individual fields. If an error is not a recognized
// validation or decode error, HasAny will return true while Get and All remain
// empty — inspect [BindErrors.Data] or [BindErrors.Params] directly in that case.
type BindErrors struct {
	// Data contains the raw error from request body decoding or validation.
	Data error
	// Params contains the raw error from parameter binding or validation.
	Params error

	dataMessages   map[string]string
	paramsMessages map[string]string
}

// HasAny returns true if there are any data or parameter binding errors.
func (b *BindErrors) HasAny() bool {
	return b.Data != nil || b.Params != nil
}

// Get returns the translated error message for the given field name. It checks
// data errors first, then parameter errors. Returns an empty string if no error
// exists for the given field. Use dot-separated paths for nested struct fields
// (e.g. "address.city").
func (b *BindErrors) Get(field string) string {
	if msg, ok := b.dataMessages[field]; ok {
		return msg
	}

	if msg, ok := b.paramsMessages[field]; ok {
		return msg
	}

	return ""
}

// All returns a flat map of all translated error messages, merging data and
// parameter errors. If a field name exists in both, the data error takes
// precedence.
func (b *BindErrors) All() map[string]string {
	result := make(map[string]string, len(b.dataMessages)+len(b.paramsMessages))

	maps.Copy(result, b.paramsMessages)
	maps.Copy(result, b.dataMessages) // Data errors take precedence.

	return result
}

// setDataError sets the data error and pre-translates it. When fn is non-nil it
// is used for validation error messages; otherwise the built-in defaults apply.
func (b *BindErrors) setDataError(err error, fn MessageFunc) {
	b.Data = err
	b.dataMessages = translateDataError(err, fn)
}

// setParamsError sets the params error and pre-translates it.
func (b *BindErrors) setParamsError(err error) {
	b.Params = err
	b.paramsMessages = translateParamsError(err)
}
