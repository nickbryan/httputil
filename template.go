package httputil

import (
	"fmt"
	"html/template"
	"io"
)

// TemplateConflictError is returned by [NewTemplateSet] when a name in the
// templates map conflicts with a template already defined in the base.
type TemplateConflictError struct {
	Name string
}

// Error implements the error interface.
func (e *TemplateConflictError) Error() string {
	return fmt.Sprintf("template %q conflicts with a template already defined in base", e.Name)
}

// TemplateUndefinedError is returned by [TemplateSet.ExecuteTemplate] when the
// named template is not found in the set.
type TemplateUndefinedError struct {
	Name string
}

// Error implements the error interface.
func (e *TemplateUndefinedError) Error() string {
	return fmt.Sprintf("template %q is undefined", e.Name)
}

// TemplateExecutor renders a named template with data. Both [*template.Template]
// and [*TemplateSet] implement this interface.
type TemplateExecutor interface {
	ExecuteTemplate(w io.Writer, name string, data any) error
}

// TemplateSet holds a collection of independently-cloned templates built from a
// shared base. Each template has its own block namespace, allowing multiple
// templates to define the same block names (e.g. "content", "title") without
// conflict.
//
// Every template parsed into the set becomes a first-class entry point — there
// is no distinction between "pages" and "partials". Any template can invoke
// layouts, other partials, or stand alone.
//
// A TemplateSet is immutable after construction and safe for concurrent use.
type TemplateSet struct {
	base      *template.Template
	templates map[string]*template.Template
}

// NewTemplateSet creates a TemplateSet by cloning a base template for each
// entry in templates and parsing the entry's source into its own clone. The base
// template should contain all shared templates (layouts, partials, helper
// definitions) that individual templates may reference.
//
// Every entry in templates gets its own clone of base, so block definitions
// (e.g. {{ define "content" }}) in one entry do not conflict with those in
// another.
//
// The base template is also cloned once with no additional parsing, so that
// templates defined directly in base (e.g. partials) are available as entry
// points too.
//
// Returns an error if any template source fails to parse or if a name in
// templates conflicts with a template already defined in base.
func NewTemplateSet(base *template.Template, templates map[string]string) (*TemplateSet, error) {
	baseClone, err := base.Clone()
	if err != nil {
		return nil, fmt.Errorf("cloning base template: %w", err)
	}

	baseNames := make(map[string]struct{})
	for _, t := range baseClone.Templates() {
		baseNames[t.Name()] = struct{}{}
	}

	clones := make(map[string]*template.Template, len(templates))

	for name, source := range templates {
		if _, exists := baseNames[name]; exists {
			return nil, &TemplateConflictError{Name: name}
		}

		// Clone the original base (not baseClone) so each entry starts
		// from the same unmodified template tree.
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("cloning base template for %q: %w", name, err)
		}

		if _, err := clone.New(name).Parse(source); err != nil {
			return nil, fmt.Errorf("parsing template %q: %w", name, err)
		}

		clones[name] = clone
	}

	return &TemplateSet{
		base:      baseClone,
		templates: clones,
	}, nil
}

// ExecuteTemplate looks up the named template in the set and executes it. The
// named template is used as the entry point — if it invokes a layout via
// {{ template "layout" . }}, the layout and its blocks are resolved within that
// template's clone.
//
// Page templates (those passed in the templates map) are checked first. If not
// found, the base clone is checked for base-defined templates (partials,
// layouts). Returns an error if the name is not found in either.
func (ts *TemplateSet) ExecuteTemplate(w io.Writer, name string, data any) error {
	if t, ok := ts.templates[name]; ok {
		if err := t.ExecuteTemplate(w, name, data); err != nil {
			return fmt.Errorf("executing template %q: %w", name, err)
		}

		return nil
	}

	if t := ts.base.Lookup(name); t != nil {
		if err := t.Execute(w, data); err != nil {
			return fmt.Errorf("executing template %q: %w", name, err)
		}

		return nil
	}

	return &TemplateUndefinedError{Name: name}
}

// Lookup returns the named template. Page templates (those passed in the
// templates map) are checked first, followed by base-defined templates
// (layouts, partials). Returns nil if the name is not found in either.
func (ts *TemplateSet) Lookup(name string) *template.Template {
	if t, ok := ts.templates[name]; ok {
		return t.Lookup(name)
	}

	return ts.base.Lookup(name)
}
