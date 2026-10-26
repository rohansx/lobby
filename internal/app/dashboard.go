package app

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "lobby_session"

func (s *Server) session(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	_, ok := s.app.SessionAdmin(r.Context(), c.Value)
	return ok
}

// gate sends visitors to /setup on a fresh install and to /login otherwise.
func (s *Server) gate(w http.ResponseWriter, r *http.Request) bool {
	if !s.app.HasAdmin(r.Context()) {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return false
	}
	if !s.session(r) {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return false
	}
	return true
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, tok string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", MaxAge: maxAge, HttpOnly: true,
		Secure: strings.HasPrefix(s.app.cfg.BaseURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	if s.app.HasAdmin(r.Context()) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, 200, "dash_setup", map[string]any{"Bare": true, "Mailer": s.app.mailer.Name(), "From": s.app.cfg.FromEmail})
}

func (s *Server) setupPost(w http.ResponseWriter, r *http.Request) {
	email, pw := r.FormValue("email"), r.FormValue("password")
	if err := s.app.CreateFirstAdmin(r.Context(), email, pw); err != nil {
		s.render(w, 400, "dash_setup", map[string]any{"Bare": true, "Error": err.Error(), "Email": email, "Mailer": s.app.mailer.Name(), "From": s.app.cfg.FromEmail})
		return
	}
	tok, err := s.app.Login(r.Context(), email, pw)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.setCookie(w, r, tok, sessionTTL)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if !s.app.HasAdmin(r.Context()) {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	s.render(w, 200, "dash_login", map[string]any{"Bare": true, "Next": r.URL.Query().Get("next")})
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	if !s.app.spam.Allow("login:"+s.clientIP(r), 10, 15*time.Minute) {
		s.render(w, 429, "dash_login", map[string]any{"Bare": true, "Error": "Too many attempts. Wait a few minutes.", "Email": email})
		return
	}
	tok, err := s.app.Login(r.Context(), email, r.FormValue("password"))
	if err != nil {
		s.render(w, 401, "dash_login", map[string]any{"Bare": true, "Error": err.Error(), "Email": email, "Next": r.FormValue("next")})
		return
	}
	s.setCookie(w, r, tok, sessionTTL)
	next := r.FormValue("next")
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.app.Logout(r.Context(), c.Value)
	}
	s.setCookie(w, r, "", -1)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type projectRow struct {
	*Project
	Stats *Stats
}

func (s *Server) dashHome(w http.ResponseWriter, r *http.Request) {
	if !s.gate(w, r) {
		return
	}
	projects, err := s.app.Projects(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var rows []projectRow
	for _, p := range projects {
		st, _ := s.app.Stats(r.Context(), p)
		rows = append(rows, projectRow{p, st})
	}
	queued, failed := s.app.QueueStats(r.Context())
	s.render(w, 200, "dash_home", map[string]any{
		"Projects": rows, "Mailer": s.app.mailer.Name(), "From": s.app.cfg.FromEmail,
		"Queued": queued, "Failed": failed, "Error": r.URL.Query().Get("error"),
	})
}

func (s *Server) dashCreateProject(w http.ResponseWriter, r *http.Request) {
	if !s.gate(w, r) {
		return
	}
	p, sk, err := s.app.CreateProject(r.Context(), r.FormValue("name"), r.FormValue("slug"))
	if err != nil {
		http.Redirect(w, r, "/?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	s.renderProject(w, r, p, sk, "")
}

// project loads the {slug} project or writes a 404.
func (s *Server) project(w http.ResponseWriter, r *http.Request) *Project {
	if !s.gate(w, r) {
		return nil
	}
	p, err := s.app.ProjectBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.NotFound(w, r)
		return nil
	}
	return p
}

func (s *Server) dashProject(w http.ResponseWriter, r *http.Request) {
	if p := s.project(w, r); p != nil {
		s.renderProject(w, r, p, "", r.URL.Query().Get("flash"))
	}
}

type chartBar struct {
	X, Y, W, H float64
	Day        string
	Count      int64
}

// dailyChart lays out a 30-day bar chart. Missing days are zero, bars are
// thin with a 2px gap, and the top of the scale is a round number.
func dailyChart(daily []DayCount) (bars []chartBar, max int64, ticks []int64) {
	counts := map[string]int64{}
	for _, d := range daily {
		counts[d.Day] = d.Count
		if d.Count > max {
			max = d.Count
		}
	}
	top := niceCeil(max)
	const width, height, gap = 600.0, 140.0, 2.0
	bw := width/30 - gap
	start := time.Now().UTC().AddDate(0, 0, -29)
	for i := 0; i < 30; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		c := counts[day]
		h := 0.0
		if top > 0 {
			h = float64(c) / float64(top) * height
		}
		if c > 0 && h < 2 {
			h = 2
		}
		bars = append(bars, chartBar{X: float64(i) * (bw + gap), Y: height - h, W: bw, H: h, Day: day, Count: c})
	}
	if top > 0 {
		ticks = []int64{0, top}
	}
	return bars, top, ticks
}

// niceCeil rounds up to 1, 2 or 5 times a power of ten.
func niceCeil(n int64) int64 {
	if n <= 0 {
		return 0
	}
	for mag := int64(1); ; mag *= 10 {
		for _, m := range []int64{1, 2, 5} {
			if m*mag >= n {
				return m * mag
			}
		}
	}
}

func (s *Server) renderProject(w http.ResponseWriter, r *http.Request, p *Project, newSecret, flash string) {
	st, err := s.app.Stats(r.Context(), p)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	bars, top, ticks := dailyChart(st.Daily)
	base := s.app.cfg.BaseURL
	s.render(w, 200, "dash_project", map[string]any{
		"Project": p, "Stats": st, "Bars": bars, "Top": top, "Ticks": ticks, "NewSecret": newSecret, "Flash": flash,
		"Waiting":   st.ByStatus[StatusConfirmed],
		"ScriptTag": `<div id="waitlist"></div>` + "\n" + `<script defer src="` + base + `/v1/wl.js" data-project="` + p.PublicKey + `" data-target="#waitlist"></script>`,
		"FormSnippet": `<form method="post" action="` + base + `/v1/w/` + p.PublicKey + `/signup">` + "\n" +
			`  <input type="email" name="email" required placeholder="you@example.com">` + "\n" +
			`  <input type="text" name="website" tabindex="-1" autocomplete="off" style="position:absolute;left:-9999px" aria-hidden="true">` + "\n" +
			`  <button type="submit">Join the waitlist</button>` + "\n" + `</form>`,
		"HostedURL": base + "/w/" + p.Slug,
		"Section":   "overview",
	})
}

func (s *Server) dashSignups(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	f := SignupFilter{Status: q.Get("status"), Query: q.Get("q"), Before: before, Limit: 50}
	list, err := s.app.ListSignups(r.Context(), p, f)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var next int64
	if len(list) == 50 {
		next = list[len(list)-1].ID
	}
	refs := s.creditedCounts(r, p)
	type row struct {
		*Signup
		Referrals int64
		Position  int64
	}
	var rows []row
	for _, su := range list {
		rr := row{Signup: su, Referrals: refs[su.ID]}
		if su.Status == StatusConfirmed {
			pl, _ := s.app.PlaceOf(r.Context(), p, su)
			rr.Position = pl.Position
		}
		rows = append(rows, rr)
	}
	s.render(w, 200, "dash_signups", map[string]any{
		"Project": p, "Rows": rows, "Filter": f, "Next": next, "Section": "signups", "Flash": q.Get("flash"),
		"Statuses": []string{StatusPending, StatusConfirmed, StatusInvited, StatusJoined, StatusRemoved},
	})
}

func (s *Server) dashSignupAction(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	r.ParseForm()
	var ids []int64
	for _, v := range r.Form["id"] {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	ctx := r.Context()
	flash := ""
	switch r.FormValue("action") {
	case "invite":
		list, err := s.app.InviteIDs(ctx, p, ids)
		if err != nil {
			flash = err.Error()
		} else {
			flash = strconv.Itoa(len(list)) + " invited (only confirmed people can be invited)"
		}
	case "joined":
		for _, id := range ids {
			s.app.MarkJoined(ctx, p, id)
		}
		flash = strconv.Itoa(len(ids)) + " marked joined"
	case "confirm":
		n := 0
		for _, id := range ids {
			if _, ok, _ := s.app.Confirm(ctx, p, id); ok {
				n++
			}
		}
		flash = strconv.Itoa(n) + " confirmed"
	case "resend":
		for _, id := range ids {
			if su, err := s.app.Signup(ctx, p.ID, id); err == nil {
				s.app.ResendStatus(ctx, p, su)
			}
		}
		flash = "Emails queued (at most one per person per 10 minutes)"
	case "remove":
		for _, id := range ids {
			s.app.Remove(ctx, p, id, false)
		}
		flash = strconv.Itoa(len(ids)) + " removed"
	case "delete":
		for _, id := range ids {
			s.app.Remove(ctx, p, id, true)
		}
		flash = strconv.Itoa(len(ids)) + " deleted and suppressed"
	}
	back := "/p/" + p.Slug + "/signups"
	if ret := r.FormValue("return"); strings.HasPrefix(ret, back) {
		back = ret
	}
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	http.Redirect(w, r, back+sep+"flash="+url.QueryEscape(flash), http.StatusSeeOther)
}

func (s *Server) dashInviteNext(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	n, _ := strconv.Atoi(r.FormValue("count"))
	list, err := s.app.InviteNext(r.Context(), p, n)
	flash := strconv.Itoa(len(list)) + " people invited"
	if err != nil {
		flash = err.Error()
	}
	http.Redirect(w, r, "/p/"+p.Slug+"?flash="+url.QueryEscape(flash), http.StatusSeeOther)
}

func (s *Server) dashExport(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	format := r.PathValue("fmt")
	if format != "csv" && format != "ndjson" {
		http.NotFound(w, r)
		return
	}
	s.export(w, r, p, format)
}

func (s *Server) dashSettings(w http.ResponseWriter, r *http.Request) {
	if p := s.project(w, r); p != nil {
		s.render(w, 200, "dash_settings", map[string]any{"Project": p, "Section": "settings", "Flash": r.URL.Query().Get("flash")})
	}
}

func (s *Server) dashSettingsPost(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	atoi := func(k string, def int) int {
		if n, err := strconv.Atoi(r.FormValue(k)); err == nil && n >= 0 {
			return n
		}
		return def
	}
	p.Name = strings.TrimSpace(r.FormValue("name"))
	p.Headline = strings.TrimSpace(r.FormValue("headline"))
	p.Description = strings.TrimSpace(r.FormValue("description"))
	p.SiteURL = strings.TrimSpace(r.FormValue("site_url"))
	p.RedirectURL = strings.TrimSpace(r.FormValue("redirect_url"))
	p.InviteURL = strings.TrimSpace(r.FormValue("invite_url"))
	p.AllowedOrigins = r.FormValue("allowed_origins")
	p.DOIEnabled = r.FormValue("doi_enabled") == "on"
	p.ShowPosition = r.FormValue("show_position") == "on"
	p.PositionMode = r.FormValue("position_mode")
	p.PointsPerReferral = atoi("points_per_referral", p.PointsPerReferral)
	p.ReferralIPCap = atoi("referral_ip_cap", p.ReferralIPCap)
	p.ReferralHourlyCap = atoi("referral_hourly_cap", p.ReferralHourlyCap)
	p.CounterFloor = atoi("counter_floor", p.CounterFloor)
	p.FromName = strings.TrimSpace(r.FormValue("from_name"))
	p.ReplyTo = strings.TrimSpace(r.FormValue("reply_to"))
	p.ConsentText = strings.TrimSpace(r.FormValue("consent_text"))
	if p.Name == "" {
		s.render(w, 400, "dash_settings", map[string]any{"Project": p, "Section": "settings", "Error": "Name is required."})
		return
	}
	if err := s.app.UpdateProject(r.Context(), p); err != nil {
		s.render(w, 400, "dash_settings", map[string]any{"Project": p, "Section": "settings", "Error": err.Error()})
		return
	}
	http.Redirect(w, r, "/p/"+p.Slug+"/settings?flash=Saved", http.StatusSeeOther)
}

func (s *Server) dashRotate(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	sk, err := s.app.RotateSecret(r.Context(), p.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.renderProject(w, r, p, sk, "Secret key rotated. The old key stopped working just now.")
}

func (s *Server) dashDelete(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	if r.FormValue("confirm") != p.Slug {
		s.render(w, 400, "dash_settings", map[string]any{"Project": p, "Section": "settings", "Error": "Type the slug to confirm deletion."})
		return
	}
	s.app.DeleteProject(r.Context(), p.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) dashWebhooks(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	hooks, _ := s.app.Webhooks(r.Context(), p)
	dels, _ := s.app.Deliveries(r.Context(), p, 50)
	s.render(w, 200, "dash_webhooks", map[string]any{
		"Project": p, "Hooks": hooks, "Deliveries": dels, "Events": Events, "Section": "webhooks",
		"Flash": r.URL.Query().Get("flash"), "Error": r.URL.Query().Get("error"),
	})
}

func (s *Server) dashWebhookPost(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	var err error
	flash := ""
	switch r.FormValue("action") {
	case "create":
		r.ParseForm()
		_, err = s.app.CreateWebhook(ctx, p, strings.TrimSpace(r.FormValue("url")), r.FormValue("kind"), r.Form["events"])
		flash = "Webhook added"
	case "test":
		err = s.app.TestWebhook(ctx, p, id)
		flash = "Test event queued"
	case "enable":
		err = s.app.SetWebhookActive(ctx, p, id, true)
		flash = "Enabled"
	case "disable":
		err = s.app.SetWebhookActive(ctx, p, id, false)
		flash = "Disabled"
	case "delete":
		err = s.app.DeleteWebhook(ctx, p, id)
		flash = "Deleted"
	case "redeliver":
		err = s.app.Redeliver(ctx, p, id)
		flash = "Redelivery queued"
	default:
		err = errors.New("unknown action")
	}
	target := "/p/" + p.Slug + "/webhooks?"
	if err != nil {
		target += "error=" + url.QueryEscape(err.Error())
	} else {
		target += "flash=" + url.QueryEscape(flash)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) dashBroadcast(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	list, _ := s.app.Broadcasts(r.Context(), p)
	st, _ := s.app.Stats(r.Context(), p)
	s.render(w, 200, "dash_broadcast", map[string]any{
		"Project": p, "Broadcasts": list, "Stats": st, "Section": "broadcast", "Flash": r.URL.Query().Get("flash"),
	})
}

func (s *Server) dashBroadcastPost(w http.ResponseWriter, r *http.Request) {
	p := s.project(w, r)
	if p == nil {
		return
	}
	n, err := s.app.Broadcast(r.Context(), p, r.FormValue("subject"), r.FormValue("body"), r.FormValue("audience"))
	if err != nil {
		list, _ := s.app.Broadcasts(r.Context(), p)
		st, _ := s.app.Stats(r.Context(), p)
		s.render(w, 400, "dash_broadcast", map[string]any{
			"Project": p, "Broadcasts": list, "Stats": st, "Section": "broadcast", "Error": err.Error(),
			"Subject": r.FormValue("subject"), "Body": r.FormValue("body"),
		})
		return
	}
	http.Redirect(w, r, "/p/"+p.Slug+"/broadcast?flash="+url.QueryEscape("Queued for "+strconv.Itoa(n)+" people"), http.StatusSeeOther)
}
