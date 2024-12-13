package problem

import (
	"encoding/json"
	"net/http"
)

const errDocLocation = "https://pkg.go.dev/github.com/nickbryan/httputil/problem#"

type Details struct {
	Type             string
	Title            string
	Detail           string
	Status           int
	Instance         string
	ExtensionMembers map[string]any
}

// MarshalJSON implements the json.Marshaler interface for Problem.
func (d *Details) MarshalJSON() ([]byte, error) {
	deets := make(map[string]any)

	deets["type"] = d.Type
	deets["title"] = d.Title
	deets["detail"] = d.Detail
	deets["status"] = d.Status
	deets["instance"] = d.Instance

	for k, v := range d.ExtensionMembers {
		deets[k] = v
	}

	return json.Marshal(deets)
}

func (p *Details) Error() string { return p.Type }

type Field struct {
	Detail  string `json:"detail"`
	Pointer string `json:"pointer"`
}

func ConstraintViolation(instance string, fields ...Field) error {
	return &Details{
		Type:             errDocLocation + "ConstraintViolation",
		Title:            "Constraint Violation",
		Detail:           "The request data violated one or more validation constraints.",
		Status:           http.StatusBadRequest,
		Instance:         instance,
		ExtensionMembers: map[string]any{"violations": fields},
	}
}
