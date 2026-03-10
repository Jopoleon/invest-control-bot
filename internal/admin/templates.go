package admin

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

// renderer is a thin wrapper around html/template for admin pages.
type renderer struct {
	tpl *template.Template
}

// newRenderer parses embedded admin templates once during startup.
func newRenderer() (*renderer, error) {
	tpl, err := template.New("admin").ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse admin templates: %w", err)
	}
	return &renderer{tpl: tpl}, nil
}

// render executes named template with provided data model.
func (r *renderer) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.tpl.ExecuteTemplate(w, name, data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("template render error"))
	}
}
