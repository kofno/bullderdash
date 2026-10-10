package web

import (
	_ "embed"
	"net/http"
)

//go:embed assets/console.html
var consoleHTML []byte

// ConsoleHandler serves the single-file, same-origin search console. The asset
// is compiled into the binary (go:embed) so there are no external files or CDN
// dependencies at runtime; it drives entirely off the /v1/search and
// /v1/jobs/{id} JSON endpoints exposed by SearchAPI.
func ConsoleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(consoleHTML)
	}
}
