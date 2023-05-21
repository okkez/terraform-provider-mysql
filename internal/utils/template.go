package utils

import (
	"bytes"
	"text/template"
)

func Render(source string, data any) (string, error) {
	t, err := template.New("main.tf").Parse(source)
	if err != nil {
		return "", err
	}
	w := new(bytes.Buffer)
	err = t.Execute(w, data)
	if err != nil {
		return "", err
	}
	return string(w.Bytes()), nil
}
