package admin

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var assetsFS embed.FS

// staticHandler serves local admin assets (css/js) bundled into the binary.
func staticHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}

// faviconHandler serves one shared site icon for both admin and public pages.
// Browsers request /favicon.ico implicitly, so the route is intentionally not
// scoped under /admin/assets even though the embedded file lives there.
func faviconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, assetsFS, "assets/img/favicon.png")
	})
}
