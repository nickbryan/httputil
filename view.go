package httputil

import (
	"fmt"
	"html/template"
	"io/fs"
	"strings"
)

// ReadHTMLTemplates parses HTML templates from the given file system and organizes
// them under a "views" template collection. It recursively walks through
// `fsys`, reading and parsing `.html` files into the template. It returns the
// compiled template or an error if file reading or parsing fails.
func ReadHTMLTemplates(fsys fs.ReadFileFS) (*template.Template, error) {
	views := template.New("views")

	if err := fs.WalkDir(fsys, "views", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".html") {
			content, readErr := fsys.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			// TODO: Can this be done if we have to trim the prefix, how do we know the prefix is "views/"?
			_, parseErr := views.New(strings.TrimPrefix(path, "views/")).Parse(string(content))
			if parseErr != nil {
				return parseErr
			}
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking views: %w", err)
	}

	return views, nil
}
