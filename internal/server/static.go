package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

func webFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// spaHandler serves the embedded Vite build. Requests under /assets/ are served
// as-is; any other GET that does not match a real file falls back to index.html
// so React Router routes (e.g. /admin) work on a hard refresh.
type spaHandler struct {
	files http.FileSystem
}

func newSPAHandler() (http.Handler, error) {
	sub, err := webFS()
	if err != nil {
		return nil, err
	}
	return &spaHandler{files: http.FS(sub)}, nil
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}
	upath = path.Clean(upath)

	// Try the requested file; if it doesn't exist, fall back to index.html.
	f, err := h.files.Open(upath)
	if err == nil {
		stat, statErr := f.Stat()
		f.Close()
		if statErr == nil && !stat.IsDir() {
			http.FileServer(h.files).ServeHTTP(w, r)
			return
		}
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	http.FileServer(h.files).ServeHTTP(w, r2)
}
