package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The dashboard is a Vite single-page app (see /web) talking to these JSON
// endpoints under /api/admin, authenticated by the session cookie. Cross-site
// writes are refused by http.CrossOriginProtection in Handler.

const sessionCookie = "lobby_session"

type adminKey struct{}

func (s *Server) setCookie(w http.ResponseWriter, tok string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", MaxAge: maxAge, HttpOnly: true,
		Secure: strings.HasPrefix(s.app.cfg.BaseURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "sign in")
			return
		}
		id, ok := s.app.SessionAdmin(r.Context(), c.Value)
		if !ok {
			jsonError(w, http.StatusUnauthorized, "sign in")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminKey{}, id)))
	})
}

func adminID(r *http.Request) int64 { return r.Context().Value(adminKey{}).(int64) }

// adminProject loads the {slug} project for an /api/admin/projects/{slug}/... route.
// In cloud mode another account's project reads as not found.
func (s *Server) adminProject(w http.ResponseWriter, r *http.Request) *Project {
	p, err := s.app.ProjectBySlug(r.Context(), r.PathValue("slug"))
	if err == nil && s.app.cfg.Cloud && p.OwnerID != adminID(r) {
		err = ErrNotFound
	}
	if err != nil {
		jsonError(w, http.StatusNotFound, "project not found")
		return nil
	}
	return p
}

// Session state, readable without a session so the app knows whether to show
// setup, login or the dashboard.
func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	cloud := s.app.cfg.Cloud
	out := map[string]any{"setup": !cloud && !s.app.HasAdmin(r.Context()), "cloud": cloud, "mailer": s.app.mailer.Name(), "from": s.app.cfg.FromEmail, "base_url": s.app.cfg.BaseURL}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if id, ok := s.app.SessionAdmin(r.Context(), c.Value); ok {
			if ac, err := s.app.Account(r.Context(), id); err == nil {
				out["email"] = ac.Email
				out["verified"] = ac.Verified || !cloud
				out["plan"] = s.app.PlanOf(ac)
			}
		}
	}
	writeJSON(w, 200, out)
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) adminSetup(w http.ResponseWriter, r *http.Request) {
	if s.app.cfg.Cloud {
		jsonError(w, 404, "not found")
		return
	}
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if err := s.app.CreateFirstAdmin(r.Context(), c.Email, c.Password); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	s.startSession(w, r, c)
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if !s.app.spam.Allow("login:"+s.clientIP(r), 10, 15*time.Minute) {
		jsonError(w, 429, "Too many attempts. Wait a few minutes.")
		return
	}
	s.startSession(w, r, c)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, c credentials) {
	tok, err := s.app.Login(r.Context(), c.Email, c.Password)
	if err != nil {
		jsonError(w, 401, err.Error())
		return
	}
	s.setCookie(w, tok, sessionTTL)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.app.Logout(r.Context(), c.Value)
	}
	s.setCookie(w, "", -1)
	writeJSON(w, 200, map[string]any{"ok": true})
}

type projectSummary struct {
	*Project
	Total     int64 `json:"total"`
	Confirmed int64 `json:"confirmed"`
	Today     int64 `json:"today"`
}

func (s *Server) adminProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.app.Projects(r.Context(), adminID(r))
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	today := time.Now().UTC().Format("2006-01-02")
	out := []projectSummary{}
	for _, p := range projects {
		st, err := s.app.Stats(r.Context(), p)
		if err != nil {
			continue
		}
		ps := projectSummary{Project: p, Total: st.Total, Confirmed: st.ByStatus[StatusConfirmed]}
		for _, d := range st.Daily {
			if d.Day == today {
				ps.Today = d.Count
			}
		}
		out = append(out, ps)
	}
	queued, failed := s.app.QueueStats(r.Context(), adminID(r))
	writeJSON(w, 200, map[string]any{"data": out, "queued": queued, "failed": failed})
}

func (s *Server) adminCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name, Slug string }
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if err := s.app.checkProjectLimit(r.Context(), adminID(r)); err != nil {
		limitError(w, err)
		return
	}
	p, sk, err := s.app.CreateProject(r.Context(), adminID(r), body.Name, body.Slug)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"project": p, "secret_key": sk})
}

func (s *Server) adminGetProject(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	st, err := s.app.Stats(r.Context(), p)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	base := s.app.cfg.BaseURL
	writeJSON(w, 200, map[string]any{
		"project": p, "stats": st,
		"snippets": map[string]string{
			"script": `<div id="waitlist"></div>` + "\n" + `<script defer src="` + base + `/v1/wl.js" data-project="` + p.PublicKey + `" data-target="#waitlist"></script>`,
			"form": `<form method="post" action="` + base + `/v1/w/` + p.PublicKey + `/signup">` + "\n" +
				`  <input type="email" name="email" required placeholder="you@example.com">` + "\n" +
				`  <input type="text" name="website" tabindex="-1" autocomplete="off" style="position:absolute;left:-9999px" aria-hidden="true">` + "\n" +
				`  <button type="submit">Join the waitlist</button>` + "\n" + `</form>`,
			"hosted": base + "/w/" + p.Slug,
		},
	})
}

func (s *Server) adminUpdateProject(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	// Decode over the loaded project so omitted fields keep their values; the
	// identity fields are restored afterwards so they can't be changed here.
	id, slug, pk, hash, created, consent := p.ID, p.Slug, p.PublicKey, p.SecretKeyHash, p.CreatedAt, p.ConsentVersion
	if err := decodeJSON(r, p); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	p.ID, p.Slug, p.PublicKey, p.SecretKeyHash, p.CreatedAt, p.ConsentVersion = id, slug, pk, hash, created, consent
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		jsonError(w, 400, "Name is required.")
		return
	}
	for _, n := range []int{p.PointsPerReferral, p.ReferralIPCap, p.ReferralHourlyCap, p.CounterFloor} {
		if n < 0 {
			jsonError(w, 400, "Numbers can't be negative.")
			return
		}
	}
	if err := s.app.UpdateProject(r.Context(), p); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	p, _ = s.app.ProjectByID(r.Context(), id)
	writeJSON(w, 200, p)
}

func (s *Server) adminRotate(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	sk, err := s.app.RotateSecret(r.Context(), p.ID)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"secret_key": sk})
}

func (s *Server) adminDeleteProject(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	if r.URL.Query().Get("confirm") != p.Slug {
		jsonError(w, 400, "Type the slug to confirm deletion.")
		return
	}
	if err := s.app.DeleteProject(r.Context(), p.ID); err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type signupRow struct {
	*Signup
	Referrals int64  `json:"referrals"`
	Position  int64  `json:"position"`
	StatusURL string `json:"status_url"`
}

func (s *Server) adminSignups(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	const limit = 50
	list, err := s.app.ListSignups(r.Context(), p, SignupFilter{Status: q.Get("status"), Query: q.Get("q"), Before: before, Limit: limit})
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	refs := s.creditedCounts(r, p)
	rows := []signupRow{}
	for _, su := range list {
		row := signupRow{Signup: su, Referrals: refs[su.ID], StatusURL: s.app.cfg.BaseURL + "/s/" + s.app.StatusToken(su)}
		if su.Status == StatusConfirmed {
			if pl, err := s.app.PlaceOf(r.Context(), p, su); err == nil {
				row.Position = pl.Position
			}
		}
		rows = append(rows, row)
	}
	var next int64
	if len(list) == limit {
		next = list[len(list)-1].ID
	}
	writeJSON(w, 200, map[string]any{"data": rows, "next": next})
}

func (s *Server) adminSignupAction(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	var body struct {
		Action string  `json:"action"`
		IDs    []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	ctx := r.Context()
	n := 0
	var msg string
	switch body.Action {
	case "invite":
		list, err := s.app.InviteIDs(ctx, p, body.IDs)
		if err != nil {
			limitError(w, err)
			return
		}
		n = len(list)
		msg = strconv.Itoa(n) + " invited"
		if n < len(body.IDs) {
			msg += ". The rest were skipped: only confirmed people can be invited."
		}
	case "joined":
		for _, id := range body.IDs {
			if _, err := s.app.MarkJoined(ctx, p, id); err == nil {
				n++
			}
		}
		msg = strconv.Itoa(n) + " marked joined"
	case "confirm":
		for _, id := range body.IDs {
			if _, ok, _ := s.app.Confirm(ctx, p, id); ok {
				n++
			}
		}
		msg = strconv.Itoa(n) + " confirmed"
	case "resend":
		for _, id := range body.IDs {
			if su, err := s.app.Signup(ctx, p.ID, id); err == nil && s.app.ResendStatus(ctx, p, su) == nil {
				n++
			}
		}
		msg = "Emails queued, at most one per person every 10 minutes"
	case "remove", "delete":
		for _, id := range body.IDs {
			if s.app.Remove(ctx, p, id, body.Action == "delete") == nil {
				n++
			}
		}
		msg = strconv.Itoa(n) + " removed"
		if body.Action == "delete" {
			msg = strconv.Itoa(n) + " deleted and suppressed"
		}
	default:
		jsonError(w, 400, "unknown action")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": n, "message": msg})
}

func (s *Server) adminInviteNext(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	var body struct {
		Count int `json:"count"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	list, err := s.app.InviteNext(r.Context(), p, body.Count)
	if err != nil {
		limitError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(list)})
}

func (s *Server) adminExport(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	format := r.PathValue("fmt")
	if format != "csv" && format != "ndjson" {
		jsonError(w, 404, "unknown format")
		return
	}
	s.export(w, r, p, format)
}

func (s *Server) adminWebhooks(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	hooks, err := s.app.Webhooks(r.Context(), p)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	dels, err := s.app.Deliveries(r.Context(), p, 50)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if hooks == nil {
		hooks = []*Webhook{}
	}
	if dels == nil {
		dels = []*Delivery{}
	}
	writeJSON(w, 200, map[string]any{"webhooks": hooks, "deliveries": dels, "events": Events})
}

func (s *Server) adminCreateWebhook(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	var body struct {
		URL    string   `json:"url"`
		Kind   string   `json:"kind"`
		Events []string `json:"events"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if plan, _ := s.app.planForOwner(r.Context(), p.OwnerID); !plan.Webhooks {
		limitError(w, &ErrLimit{"Webhooks come with the Launch plan. Upgrade to add one."})
		return
	}
	wh, err := s.app.CreateWebhook(r.Context(), p, strings.TrimSpace(body.URL), body.Kind, body.Events)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, wh)
}

func (s *Server) adminWebhookAction(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	id, ok := pathID(r)
	if !ok {
		jsonError(w, 404, "not found")
		return
	}
	ctx := r.Context()
	var err error
	switch r.PathValue("action") {
	case "test":
		err = s.app.TestWebhook(ctx, p, id)
	case "enable":
		err = s.app.SetWebhookActive(ctx, p, id, true)
	case "disable":
		err = s.app.SetWebhookActive(ctx, p, id, false)
	case "delete":
		err = s.app.DeleteWebhook(ctx, p, id)
	default:
		err = errors.New("unknown action")
	}
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) adminRedeliver(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	id, _ := pathID(r)
	if err := s.app.Redeliver(r.Context(), p, id); err != nil {
		jsonError(w, 404, "not found")
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true})
}

func (s *Server) adminBroadcasts(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	list, err := s.app.Broadcasts(r.Context(), p)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if list == nil {
		list = []Broadcast{}
	}
	st, _ := s.app.Stats(r.Context(), p)
	writeJSON(w, 200, map[string]any{"data": list, "by_status": st.ByStatus})
}

func (s *Server) adminBroadcast(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	var body struct{ Subject, Body, Audience string }
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	n, err := s.app.Broadcast(r.Context(), p, body.Subject, body.Body, body.Audience)
	if err != nil {
		limitError(w, err)
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "recipients": n})
}

// limitError answers 402 for a plan limit, so the dashboard can offer an
// upgrade, and 400 for anything else.
func limitError(w http.ResponseWriter, err error) {
	var le *ErrLimit
	if errors.As(err, &le) {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{"ok": false, "error": le.Msg, "upgrade": true})
		return
	}
	jsonError(w, 400, err.Error())
}
