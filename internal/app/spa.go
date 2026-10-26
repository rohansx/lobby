package app

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// web/dist is the Vite build of the dashboard (npm run build in /web).
//
//go:embed all:web/dist
var distFS embed.FS

type asset struct {
	body, gz []byte
	ctype    string
}

// spa serves the dashboard's static files, with index.html for every other
// path so client-side routes survive a reload. Text assets are gzipped once at
// startup. Vite's hashed assets are cached for a year; index.html is never
// cached so a deploy is picked up at once.
func (s *Server) spa() http.Handler {
	dist, _ := fs.Sub(distFS, "web/dist")
	assets := map[string]*asset{}
	fs.WalkDir(dist, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		b, err := fs.ReadFile(dist, name)
		if err != nil {
			return nil
		}
		a := &asset{body: b, ctype: mime.TypeByExtension(path.Ext(name))}
		if strings.HasPrefix(a.ctype, "text/") || strings.Contains(a.ctype, "javascript") || strings.Contains(a.ctype, "svg") || strings.Contains(a.ctype, "json") {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			zw.Write(b)
			zw.Close()
			if buf.Len() < len(b) {
				a.gz = buf.Bytes()
			}
		}
		assets[name] = a
		return nil
	})
	if assets["index.html"] == nil {
		assets["index.html"] = &asset{
			body:  []byte("<!doctype html><title>Lobby</title><p>The dashboard isn't built. Run <code>npm ci &amp;&amp; npm run build</code> in <code>web/</code>, then rebuild the binary."),
			ctype: "text/html; charset=utf-8",
		}
	}
	started := time.Now()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		a := assets[name]
		h := w.Header()
		switch {
		case a != nil && name != "index.html":
			if strings.HasPrefix(name, "assets/") {
				h.Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				h.Set("Cache-Control", "public, max-age=3600")
			}
		case strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "api/"):
			http.NotFound(w, r)
			return
		default:
			a = assets["index.html"]
			h.Set("Cache-Control", "no-cache")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("X-Frame-Options", "DENY")
		}
		h.Set("Content-Type", a.ctype)
		body := a.body
		if a.gz != nil {
			h.Add("Vary", "Accept-Encoding")
			if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				h.Set("Content-Encoding", "gzip")
				body = a.gz
			}
		}
		http.ServeContent(w, r, "", started, bytes.NewReader(body))
	})
}
