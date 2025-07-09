package httputil

import (
	"fmt"
	"html/template"
	"io/fs"
	"path/filepath"
	"strings"
)

// TemplateFSReadError represents an error encountered while reading a file or
// directory from the filesystem.
type TemplateFSReadError struct {
	// DirEntry contains the filesystem directory
	// entry related to the error.
	DirEntry fs.DirEntry
	// Path specifies the path of the file or directory
	// that caused the error.
	Path string
	// Err holds the underlying error that occurred during
	// the filesystem operation.
	Err error
}

// Error returns a string representation of the error.
func (e *TemplateFSReadError) Error() string {
	return fmt.Sprintf("reading [%s]: %s", e.Path, e.Err.Error())
}

// ReadHTMLTemplates parses HTML templates from the given file system, starting
// from the specified rootDir. It recursively walks through `fsys`, reading
// and parsing `.html` files. It returns the compiled template or an error if
// file reading or parsing fails.
func ReadHTMLTemplates(fsys fs.ReadFileFS, rootDir string) (*template.Template, error) {
	views := template.New(rootDir)
	cleanRootDir := filepath.Clean(rootDir)

	if err := fs.WalkDir(fsys, cleanRootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return &TemplateFSReadError{DirEntry: d, Path: path, Err: err}
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".html") {
			content, readErr := fsys.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("reading file: %w", readErr)
			}

			templateName := strings.TrimPrefix(path, cleanRootDir+"/")

			_, parseErr := views.New(templateName).Parse(string(content))
			if parseErr != nil {
				return fmt.Errorf("parsing template: %w", parseErr)
			}
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking templates: %w", err)
	}

	return views, nil
}
