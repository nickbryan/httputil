package httputil_test

import (
	"bytes"
	"errors"
	"html/template"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/nickbryan/httputil"
)

func TestTemplateSet_BasicRendering(t *testing.T) {
	t.Parallel()

	base := template.Must(template.New("").Parse(""))
	template.Must(base.New("layout").Parse(
		`<html><head><title>{{ block "title" . }}Default{{ end }}</title></head>` +
			`<body>{{ block "content" . }}{{ end }}</body></html>`))

	ts, err := httputil.NewTemplateSet(base, map[string]string{
		"home": `{{ template "layout" . }}{{ define "title" }}Home{{ end }}{{ define "content" }}<h1>Welcome</h1>{{ end }}`,
	})
	if err != nil {
		t.Fatalf("NewTemplateSet() error = %v", err)
	}

	var buf bytes.Buffer
	if err := ts.ExecuteTemplate(&buf, "home", nil); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	want := `<html><head><title>Home</title></head><body><h1>Welcome</h1></body></html>`
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Errorf("rendered output mismatch (-want +got):\n%s", diff)
	}
}

func TestTemplateSet_MultiplePagesWithSameBlockNames(t *testing.T) {
	t.Parallel()

	base := template.Must(template.New("").Parse(""))
	template.Must(base.New("layout").Parse(
		`<div>{{ block "content" . }}{{ end }}</div>`))

	ts, err := httputil.NewTemplateSet(base, map[string]string{
		"page-a": `{{ template "layout" . }}{{ define "content" }}Page A{{ end }}`,
		"page-b": `{{ template "layout" . }}{{ define "content" }}Page B{{ end }}`,
	})
	if err != nil {
		t.Fatalf("NewTemplateSet() error = %v", err)
	}

	var bufA bytes.Buffer
	if err := ts.ExecuteTemplate(&bufA, "page-a", nil); err != nil {
		t.Fatalf("ExecuteTemplate(page-a) error = %v", err)
	}

	var bufB bytes.Buffer
	if err := ts.ExecuteTemplate(&bufB, "page-b", nil); err != nil {
		t.Fatalf("ExecuteTemplate(page-b) error = %v", err)
	}

	if diff := cmp.Diff("<div>Page A</div>", bufA.String()); diff != "" {
		t.Errorf("page-a mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff("<div>Page B</div>", bufB.String()); diff != "" {
		t.Errorf("page-b mismatch (-want +got):\n%s", diff)
	}
}

func TestTemplateSet_PartialAsEntryPoint(t *testing.T) {
	t.Parallel()

	base := template.Must(template.New("").Parse(""))
	template.Must(base.New("partials/nav.html").Parse(`<nav>Home | About</nav>`))

	ts, err := httputil.NewTemplateSet(base, map[string]string{
		"page": `{{ template "partials/nav.html" . }}<main>Page</main>`,
	})
	if err != nil {
		t.Fatalf("NewTemplateSet() error = %v", err)
	}

	var buf bytes.Buffer
	if err := ts.ExecuteTemplate(&buf, "partials/nav.html", nil); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	if diff := cmp.Diff("<nav>Home | About</nav>", buf.String()); diff != "" {
		t.Errorf("partial output mismatch (-want +got):\n%s", diff)
	}
}

func TestTemplateSet_PageReferencingPartial(t *testing.T) {
	t.Parallel()

	base := template.Must(template.New("").Parse(""))
	template.Must(base.New("layout").Parse(
		`<html>{{ block "content" . }}{{ end }}</html>`))
	template.Must(base.New("partials/foo.html").Parse(`<p>partial: {{ . }}</p>`))

	ts, err := httputil.NewTemplateSet(base, map[string]string{
		"page": `{{ template "layout" . }}{{ define "content" }}{{ template "partials/foo.html" . }}{{ end }}`,
	})
	if err != nil {
		t.Fatalf("NewTemplateSet() error = %v", err)
	}

	var buf bytes.Buffer
	if err := ts.ExecuteTemplate(&buf, "page", "hello"); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	if diff := cmp.Diff("<html><p>partial: hello</p></html>", buf.String()); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}

func TestTemplateSet_UnknownTemplateName(t *testing.T) {
	t.Parallel()

	base := template.Must(template.New("").Parse(""))

	ts, err := httputil.NewTemplateSet(base, nil)
	if err != nil {
		t.Fatalf("NewTemplateSet() error = %v", err)
	}

	err = ts.ExecuteTemplate(&bytes.Buffer{}, "nonexistent", nil)

	var undefinedErr *httputil.TemplateUndefinedError
	if !errors.As(err, &undefinedErr) {
		t.Fatalf("ExecuteTemplate() error = %v, want TemplateUndefinedError", err)
	}

	if undefinedErr.Name != "nonexistent" {
		t.Errorf("TemplateUndefinedError.Name = %q, want %q", undefinedErr.Name, "nonexistent")
	}
}

func TestTemplateSet_NameConflict(t *testing.T) {
	t.Parallel()

	base := template.Must(template.New("").Parse(""))
	template.Must(base.New("shared").Parse(`<p>shared</p>`))

	_, err := httputil.NewTemplateSet(base, map[string]string{
		"shared": `<p>conflict</p>`,
	})

	var conflictErr *httputil.TemplateConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("NewTemplateSet() error = %v, want TemplateConflictError", err)
	}

	if conflictErr.Name != "shared" {
		t.Errorf("TemplateConflictError.Name = %q, want %q", conflictErr.Name, "shared")
	}
}

func TestTemplateSet_TemplateFunctions(t *testing.T) {
	t.Parallel()

	funcs := template.FuncMap{
		"upper": strings.ToUpper,
	}

	base := template.Must(template.New("").Funcs(funcs).Parse(""))
	template.Must(base.New("partial").Parse(`{{ upper . }}`))

	ts, err := httputil.NewTemplateSet(base, map[string]string{
		"page": `{{ upper . }}`,
	})
	if err != nil {
		t.Fatalf("NewTemplateSet() error = %v", err)
	}

	// Verify function works in page clone.
	var pageBuf bytes.Buffer
	if err := ts.ExecuteTemplate(&pageBuf, "page", "hello"); err != nil {
		t.Fatalf("ExecuteTemplate(page) error = %v", err)
	}

	if diff := cmp.Diff("HELLO", pageBuf.String()); diff != "" {
		t.Errorf("page output mismatch (-want +got):\n%s", diff)
	}

	// Verify function works in base clone.
	var partialBuf bytes.Buffer
	if err := ts.ExecuteTemplate(&partialBuf, "partial", "world"); err != nil {
		t.Fatalf("ExecuteTemplate(partial) error = %v", err)
	}

	if diff := cmp.Diff("WORLD", partialBuf.String()); diff != "" {
		t.Errorf("partial output mismatch (-want +got):\n%s", diff)
	}
}
