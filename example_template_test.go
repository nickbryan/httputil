package httputil_test

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/nickbryan/httputil"
)

//go:embed testdata/templates
var templateFS embed.FS

// ExampleNewTemplateSet_fromDisk demonstrates how to load templates from a
// nested directory structure into a [httputil.TemplateSet] and wire the result
// into an [httputil.HTMLServerCodec] with a custom error page.
//
// Layouts and partials are parsed into a shared base template, while page
// templates (including the error page) are passed as source strings so each
// page gets its own isolated clone.
//
// Directory structure:
//
//	templates/
//	  layouts/
//	    base.html       <- shared layout with {{ block "content" . }}
//	  partials/
//	    nav.html        <- reusable partial
//	  pages/
//	    home.html       <- defines "content" block
//	    about.html      <- defines "content" block (no conflict with home)
//	    error.html      <- error page, also defines "content" block
func ExampleNewTemplateSet_fromDisk() {
	// In production, use //go:embed templates to embed files at compile time.
	// Here we use the test-embedded filesystem for the same effect.
	fsys, err := fs.Sub(templateFS, "testdata/templates")
	if err != nil {
		log.Fatal(err)
	}

	// Step 1: Parse layouts and partials into the base template. These are
	// shared across all pages and must use path-based names (e.g.
	// "layouts/base.html") so that page templates can reference them.
	base := template.New("")

	if err := parseDir(base, fsys, "layouts"); err != nil {
		log.Fatal(err)
	}

	if err := parseDir(base, fsys, "partials"); err != nil {
		log.Fatal(err)
	}

	// Step 2: Read page template sources into a map. Each page will be cloned
	// from the base, so block definitions like {{ define "content" }} in one
	// page do not conflict with another.
	pages, err := readDir(fsys, "pages")
	if err != nil {
		log.Fatal(err)
	}

	// Step 3: Build the TemplateSet.
	ts, err := httputil.NewTemplateSet(base, pages)
	if err != nil {
		log.Fatal(err)
	}

	// Step 4: Create the codec. The error page is a page template like any
	// other - use Lookup to retrieve it and pass it to WithHTMLErrorTemplate.
	_ = httputil.NewHTMLServerCodec(ts,
		httputil.WithHTMLErrorTemplate(ts.Lookup("error")),
	)

	// Render each page to verify isolation.
	var buf bytes.Buffer

	if err := ts.ExecuteTemplate(&buf, "home", map[string]string{"Name": "Nick"}); err != nil {
		log.Fatal(err)
	}

	fmt.Println(strings.TrimSpace(buf.String()))
	fmt.Println("---")

	buf.Reset()

	if err := ts.ExecuteTemplate(&buf, "about", map[string]string{"Body": "About us."}); err != nil {
		log.Fatal(err)
	}

	fmt.Println(strings.TrimSpace(buf.String()))

	// Output:
	// <!DOCTYPE html>
	// <html lang="en">
	// <head><meta charset="utf-8"><title>Home</title></head>
	// <body>
	// <nav><a href="/">Home</a> | <a href="/about">About</a></nav>
	// <main><h1>Welcome, Nick!</h1></main>
	// </body>
	// </html>
	// ---
	// <!DOCTYPE html>
	// <html lang="en">
	// <head><meta charset="utf-8"><title>About</title></head>
	// <body>
	// <nav><a href="/">Home</a> | <a href="/about">About</a></nav>
	// <main><p>About us.</p></main>
	// </body>
	// </html>
}

// parseDir walks a directory within fsys and parses each .html file into the
// given template, using its path relative to fsys as the template name (e.g.
// "layouts/base.html").
func parseDir(t *template.Template, fsys fs.FS, dir string) error {
	return fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".html" {
			return err
		}

		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		if _, err := t.New(path).Parse(string(src)); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		return nil
	})
}

// readDir reads each .html file in a directory and returns a map of name to
// source string, suitable for passing to [httputil.NewTemplateSet]. The name is
// the filename without its extension (e.g. "home" from "pages/home.html").
func readDir(fsys fs.FS, dir string) (map[string]string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	pages := make(map[string]string, len(entries))

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".html" {
			continue
		}

		src, err := fs.ReadFile(fsys, filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}

		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		pages[name] = string(src)
	}

	return pages, nil
}
