package app

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var appTemplatesFS embed.FS

var appTemplates = template.Must(template.New("app").ParseFS(appTemplatesFS, "templates/*.html"))

func renderAppTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := appTemplates.ExecuteTemplate(w, name, data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("template render error: %v", err)))
	}
}
