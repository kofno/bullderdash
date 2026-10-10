package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// assetsFS holds the shared static assets (design system + JS helpers) compiled
// into the binary. Served same-origin under /assets/ so there are no CDN or
// external-file dependencies at runtime, matching the console's model.
//
//go:embed assets/app.css assets/app.js
var assetsFS embed.FS

// assetContentTypes maps extensions to explicit Content-Type values so we do not
// rely on OS mime registries (which vary on Windows/containers).
var assetContentTypes = map[string]string{
	".css": "text/css; charset=utf-8",
	".js":  "text/javascript; charset=utf-8",
}

// assetBuildID is stamped at process start and used as a weak cache-buster query
// value by callers (e.g. ?v=<id>) so a redeploy invalidates cached CSS/JS.
var assetBuildID = time.Now().UTC().Format("20060102150405")

// AssetBuildID returns the per-process asset version string.
func AssetBuildID() string { return assetBuildID }

// AssetsHandler serves the embedded static assets under /assets/. It only
// exposes files that actually exist in the embedded FS and sets long-lived,
// immutable caching (invalidated via the ?v= build id).
func AssetsHandler() http.HandlerFunc {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/assets/")
		if name == "" || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(sub, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if dot := strings.LastIndex(name, "."); dot >= 0 {
			if ct := assetContentTypes[name[dot:]]; ct != "" {
				w.Header().Set("Content-Type", ct)
			}
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}
