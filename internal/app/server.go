package app

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

//go:embed web/templates/*.html
var templateFS embed.FS

//go:embed web/static
var staticFS embed.FS

type Server struct {
	app  *App
	tmpl map[string]*template.Template
}

var funcs = template.FuncMap{
	"date": func(t int64) string { return time.Unix(t, 0).UTC().Format("2 Jan 2006") },
	"datetime": func(t int64) string {
		return time.Unix(t, 0).UTC().Format("2 Jan 2006 15:04")
	},
	"ptime": func(t *int64) string {
		if t == nil {
			return ""
		}
		return time.Unix(*t, 0).UTC().Format("2 Jan 15:04")
	},
	"pct": func(f float64) string { return fmt.Sprintf("%.0f%%", f*100) },
	"num": func(n int64) string {
		s := strconv.FormatInt(n, 10)
		for i := len(s) - 3; i > 0; i -= 3 {
			s = s[:i] + "," + s[i:]
		}
		return s
	},
	"json": func(v any) template.JS {
		b, _ := json.Marshal(v)
		return template.JS(b)
	},
	"add": func(a, b int64) int64 { return a + b },
	"lines": func(s string) []string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
		return out
	},
}

func NewServer(a *App) (*Server, error) {
	s := &Server{app: a, tmpl: map[string]*template.Template{}}
	pages, err := fs.Glob(templateFS, "web/templates/*.html")
	if err != nil {
		return nil, err
	}
	for _, page := range pages {
		name := strings.TrimSuffix(strings.TrimPrefix(page, "web/templates/"), ".html")
		if strings.HasPrefix(name, "_") {
			continue
		}
		layout := "web/templates/_public.html"
		if strings.HasPrefix(name, "dash_") {
			layout = "web/templates/_dash.html"
		}
		t, err := template.New(name).Funcs(funcs).ParseFS(templateFS, layout, page)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		s.tmpl[name] = t
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "web/static")

	// Public: CORS-open, project public key in the path, no session.
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /llms.txt", s.llms)
	mux.HandleFunc("GET /v1/wl.js", s.widgetJS)
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(http.FileServerFS(static), 3600)))
	mux.HandleFunc("OPTIONS /v1/w/{pk}/signup", s.preflight)
	mux.HandleFunc("POST /v1/w/{pk}/signup", s.publicSignup)
	mux.HandleFunc("GET /v1/w/{pk}/count", s.publicCount)
	mux.HandleFunc("GET /v1/s/{token}", s.publicStatus)
	mux.HandleFunc("POST /v1/s/{token}/confirm", s.publicConfirm)
	mux.HandleFunc("GET /w/{slug}", s.hostedPage)
	mux.HandleFunc("GET /w/{slug}/thanks", s.thanksPage)
	mux.HandleFunc("GET /s/{token}", s.statusPage)
	mux.HandleFunc("POST /s/{token}/confirm", s.statusConfirm)
	mux.HandleFunc("GET /u/{token}", s.unsubscribePage)
	mux.HandleFunc("POST /u/{token}", s.unsubscribePost)
	mux.HandleFunc("POST /v1/hooks/resend", s.resendHook)

	// Admin API: project secret key as a bearer token.
	api := http.NewServeMux()
	api.HandleFunc("GET /v1/project", s.apiProject)
	api.HandleFunc("GET /v1/stats", s.apiStats)
	api.HandleFunc("GET /v1/signups", s.apiListSignups)
	api.HandleFunc("GET /v1/signups/{id}", s.apiGetSignup)
	api.HandleFunc("DELETE /v1/signups/{id}", s.apiRemoveSignup)
	api.HandleFunc("POST /v1/signups/{id}/joined", s.apiJoined)
	api.HandleFunc("POST /v1/invites", s.apiInvite)
	api.HandleFunc("POST /v1/invites/redeem", s.apiRedeem)
	api.HandleFunc("GET /v1/export.csv", s.apiExportCSV)
	api.HandleFunc("GET /v1/export.ndjson", s.apiExportNDJSON)
	api.HandleFunc("GET /v1/webhooks", s.apiListWebhooks)
	api.HandleFunc("POST /v1/webhooks", s.apiCreateWebhook)
	api.HandleFunc("DELETE /v1/webhooks/{id}", s.apiDeleteWebhook)
	api.HandleFunc("POST /v1/webhooks/{id}/test", s.apiTestWebhook)
	api.HandleFunc("GET /v1/deliveries", s.apiDeliveries)
	api.HandleFunc("POST /v1/deliveries/{id}/redeliver", s.apiRedeliver)
	api.HandleFunc("POST /v1/broadcasts", s.apiBroadcast)
	for _, p := range []string{"/v1/project", "/v1/stats", "/v1/signups", "/v1/signups/", "/v1/invites", "/v1/invites/",
		"/v1/export.csv", "/v1/export.ndjson", "/v1/webhooks", "/v1/webhooks/", "/v1/deliveries", "/v1/deliveries/", "/v1/broadcasts"} {
		mux.Handle(p, s.requireKey(api))
	}

	// Dashboard: session cookie, cross-origin POSTs rejected.
	dash := http.NewServeMux()
	dash.HandleFunc("GET /{$}", s.dashHome)
	dash.HandleFunc("GET /setup", s.setupPage)
	dash.HandleFunc("POST /setup", s.setupPost)
	dash.HandleFunc("GET /login", s.loginPage)
	dash.HandleFunc("POST /login", s.loginPost)
	dash.HandleFunc("POST /logout", s.logout)
	dash.HandleFunc("POST /p", s.dashCreateProject)
	dash.HandleFunc("GET /p/{slug}", s.dashProject)
	dash.HandleFunc("GET /p/{slug}/signups", s.dashSignups)
	dash.HandleFunc("POST /p/{slug}/signups/action", s.dashSignupAction)
	dash.HandleFunc("POST /p/{slug}/invite", s.dashInviteNext)
	dash.HandleFunc("GET /p/{slug}/export/{fmt}", s.dashExport)
	dash.HandleFunc("GET /p/{slug}/settings", s.dashSettings)
	dash.HandleFunc("POST /p/{slug}/settings", s.dashSettingsPost)
	dash.HandleFunc("POST /p/{slug}/rotate", s.dashRotate)
	dash.HandleFunc("POST /p/{slug}/delete", s.dashDelete)
	dash.HandleFunc("GET /p/{slug}/webhooks", s.dashWebhooks)
	dash.HandleFunc("POST /p/{slug}/webhooks", s.dashWebhookPost)
	dash.HandleFunc("GET /p/{slug}/broadcast", s.dashBroadcast)
	dash.HandleFunc("POST /p/{slug}/broadcast", s.dashBroadcastPost)
	mux.Handle("/", http.NewCrossOriginProtection().Handler(dash))

	return s.timing(mux)
}

type timingWriter struct {
	http.ResponseWriter
	start time.Time
	wrote bool
}

func (w *timingWriter) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		w.Header().Set("Server-Timing", fmt.Sprintf("app;dur=%.2f", float64(time.Since(w.start).Microseconds())/1000))
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *timingWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *timingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// timing adds a Server-Timing header so the signup hot path can be measured
// from the browser's network panel.
func (s *Server) timing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(&timingWriter{ResponseWriter: w, start: time.Now()}, r)
	})
}

func cacheFor(h http.Handler, secs int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(secs))
		h.ServeHTTP(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.app.cfg.TrustProxy {
		// The last entry is the one our own proxy appended; earlier ones are client-supplied.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func (s *Server) render(w http.ResponseWriter, code int, name string, data map[string]any) {
	t, ok := s.tmpl[name]
	if !ok {
		http.Error(w, "missing template "+name, 500)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["BaseURL"] = s.app.cfg.BaseURL
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		s.app.log.Error("render", "template", name, "err", err)
	}
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.app.db.R.PingContext(r.Context()); err != nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok\n"))
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
