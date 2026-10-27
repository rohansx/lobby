package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ReferralLink points at the project's own landing page when it has one (the
// widget there reads ?ref=), otherwise at the hosted page.
func (a *App) ReferralLink(p *Project, s *Signup) string {
	base := p.SiteURL
	if base == "" {
		base = a.cfg.BaseURL + "/w/" + p.Slug
	}
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if u.Path == "" {
		u.Path = "/"
	}
	q := u.Query()
	q.Set("ref", s.ReferralCode)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Server) cors(w http.ResponseWriter, r *http.Request, p *Project) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if origin == originOf(s.app.cfg.BaseURL) || p == nil || p.OriginAllowed(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		return true
	}
	return false
}

func (s *Server) preflight(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.ProjectByPublicKey(r.Context(), r.PathValue("pk"))
	if err != nil || !s.cors(w, r, p) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}

// signupFields reads a signup from JSON, urlencoded or multipart bodies.
func signupFields(r *http.Request) (map[string]string, error) {
	out := map[string]string{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var m map[string]any
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&m); err != nil {
			return nil, errors.New("invalid JSON body")
		}
		for k, v := range m {
			switch x := v.(type) {
			case string:
				out[k] = x
			case float64:
				out[k] = strconv.FormatFloat(x, 'f', -1, 64)
			}
		}
		return out, nil
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<10)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(64 << 10); err != nil {
			return nil, errors.New("invalid form body")
		}
	} else if err := r.ParseForm(); err != nil {
		return nil, errors.New("invalid form body")
	}
	for k, v := range r.PostForm {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out, nil
}

// signedFormTime lets the hosted page prove when it was rendered, for the dwell check.
func (a *App) signedFormTime() string {
	t := strconv.FormatInt(time.Now().UnixMilli(), 36)
	return t + "." + a.tokenSig("form", t)
}

func (a *App) dwellFrom(f map[string]string) int64 {
	if t := f["_t"]; t != "" {
		ts, ok := a.parseToken("form", t)
		if !ok {
			return 0 // forged timestamp: treat as a bot
		}
		return time.Now().UnixMilli() - ts
	}
	if d, err := strconv.ParseInt(f["_dwell"], 10, 64); err == nil {
		return d
	}
	return -1
}

func (s *Server) publicSignup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := s.app.ProjectByPublicKey(ctx, r.PathValue("pk"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "unknown project")
		return
	}
	asJSON := wantsJSON(r)
	if !s.cors(w, r, p) {
		jsonError(w, http.StatusForbidden, "this site is not allowed to post to this waitlist")
		return
	}
	f, err := signupFields(r)
	if err != nil {
		s.signupError(w, r, p, asJSON, http.StatusBadRequest, err.Error(), nil)
		return
	}
	ip := s.clientIP(r)
	if !s.app.spam.Allow("ip:"+strconv.FormatInt(p.ID, 10)+":"+ip, 10, 10*time.Minute) {
		s.signupError(w, r, p, asJSON, http.StatusTooManyRequests, "Too many attempts. Please try again in a few minutes.", f)
		return
	}

	source := f["source_url"]
	if source == "" {
		source = r.Header.Get("Referer")
	}
	in := SignupInput{
		Email: f["email"], Ref: f["ref"], SourceURL: source, Referrer: f["referrer"],
		UTMSource: f["utm_source"], UTMMedium: f["utm_medium"], UTMCampaign: f["utm_campaign"],
		UTMTerm: f["utm_term"], UTMContent: f["utm_content"],
		IP: ip, Honeypot: f["website"], DwellMs: s.app.dwellFrom(f),
	}
	// Plain forms do not send UTM fields; recover them and ?ref= from the page URL.
	if u, err := url.Parse(source); err == nil {
		q := u.Query()
		fill := func(dst *string, key string) {
			if *dst == "" {
				*dst = q.Get(key)
			}
		}
		fill(&in.UTMSource, "utm_source")
		fill(&in.UTMMedium, "utm_medium")
		fill(&in.UTMCampaign, "utm_campaign")
		fill(&in.UTMTerm, "utm_term")
		fill(&in.UTMContent, "utm_content")
		fill(&in.Ref, "ref")
	}

	res, err := s.app.SubmitSignup(ctx, p, in)
	var ve *ValidationError
	if errors.As(err, &ve) {
		s.signupError(w, r, p, asJSON, http.StatusUnprocessableEntity, ve.Msg, f)
		return
	} else if err != nil {
		s.app.log.Error("signup", "err", err)
		s.signupError(w, r, p, asJSON, http.StatusInternalServerError, "Something went wrong. Please try again.", f)
		return
	}

	// Only a brand-new signup on a list without double opt-in reveals a status
	// page. Every other case (pending confirm, already on the list, spam check
	// tripped) gets the same "check your inbox" answer, so the endpoint never
	// tells a stranger whether an address is on the list.
	if res.Created && res.Signup.Status == StatusConfirmed {
		tok := s.app.StatusToken(res.Signup)
		statusURL := s.app.cfg.BaseURL + "/s/" + tok
		if asJSON {
			pl, _ := s.app.PlaceOf(ctx, p, res.Signup)
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true, "confirm_required": false, "message": "You're on the list.",
				"status_url": statusURL, "token": tok, "referral_link": s.app.ReferralLink(p, res.Signup),
				"position": positionOrNil(p, pl.Position), "behind": pl.Behind,
			})
			return
		}
		http.Redirect(w, r, s.redirectTarget(r, p, f, statusURL), http.StatusSeeOther)
		return
	}
	if !res.Created && !res.Fake {
		if err := s.app.ResendStatus(ctx, p, res.Signup); err != nil {
			s.app.log.Error("resend status", "err", err)
		}
	}
	msg := "Check your inbox. We've sent you a link to your spot."
	if asJSON {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "confirm_required": true, "message": msg})
		return
	}
	http.Redirect(w, r, s.redirectTarget(r, p, f, s.app.cfg.BaseURL+"/w/"+p.Slug+"/thanks"), http.StatusSeeOther)
}

func positionOrNil(p *Project, pos int64) any {
	if !p.ShowPosition || pos == 0 {
		return nil
	}
	return pos
}

// redirectTarget honours a _redirect field only when it points at an allowed
// origin, so the form endpoint cannot be used as an open redirect.
func (s *Server) redirectTarget(r *http.Request, p *Project, f map[string]string, fallback string) string {
	if want := f["_redirect"]; want != "" && isHTTPURL(want) {
		o := originOf(want)
		allowed := o == originOf(s.app.cfg.BaseURL) || (p.SiteURL != "" && o == originOf(p.SiteURL))
		if len(p.Origins()) > 0 {
			allowed = allowed || p.OriginAllowed(o)
		} else if ref := originOf(r.Header.Get("Referer")); ref != "" && ref == o {
			allowed = true
		}
		if allowed {
			return want
		}
	}
	if p.RedirectURL != "" {
		return p.RedirectURL
	}
	return fallback
}

func (s *Server) signupError(w http.ResponseWriter, r *http.Request, p *Project, asJSON bool, code int, msg string, f map[string]string) {
	if asJSON {
		jsonError(w, code, msg)
		return
	}
	// A plain form from our own hosted page re-renders with the error inline.
	if originOf(r.Header.Get("Referer")) == originOf(s.app.cfg.BaseURL) {
		email := ""
		if f != nil {
			email = f["email"]
		}
		s.renderHosted(w, r, p, code, msg, email)
		return
	}
	s.render(w, code, "message", map[string]any{
		"Title": "That didn't work", "Message": msg, "Back": r.Header.Get("Referer"), "Project": p,
	})
}

func (s *Server) publicCount(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.ProjectByPublicKey(r.Context(), r.PathValue("pk"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "unknown project")
		return
	}
	s.cors(w, r, p)
	n, err := s.app.PublicCount(r.Context(), p)
	if err != nil {
		jsonError(w, 500, "error")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	if n < 0 {
		writeJSON(w, 200, map[string]any{"count": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"count": n})
}

func (s *Server) statusJSON(r *http.Request, p *Project, su *Signup) (map[string]any, error) {
	pl, err := s.app.PlaceOf(r.Context(), p, su)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"status": su.Status, "referrals": pl.Referrals, "referral_link": s.app.ReferralLink(p, su),
		"position": positionOrNil(p, pl.Position), "behind": pl.Behind, "waiting": pl.Total,
	}
	return out, nil
}

func (s *Server) publicStatus(w http.ResponseWriter, r *http.Request) {
	su, p, err := s.app.SignupByStatusToken(r.Context(), r.PathValue("token"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	out, err := s.statusJSON(r, p, su)
	if err != nil {
		jsonError(w, 500, "error")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) publicConfirm(w http.ResponseWriter, r *http.Request) {
	su, p, err := s.app.SignupByStatusToken(r.Context(), r.PathValue("token"))
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	su, _, err = s.app.Confirm(r.Context(), p, su.ID)
	if err != nil {
		jsonError(w, 500, "error")
		return
	}
	out, _ := s.statusJSON(r, p, su)
	writeJSON(w, 200, out)
}

func (s *Server) hostedPage(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.ProjectBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.render(w, 404, "message", map[string]any{"Title": "Not found", "Message": "There's no waitlist here."})
		return
	}
	s.renderHosted(w, r, p, 200, "", "")
}

func (s *Server) renderHosted(w http.ResponseWriter, r *http.Request, p *Project, code int, errMsg, email string) {
	count, _ := s.app.PublicCount(r.Context(), p)
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = r.FormValue("ref")
	}
	s.render(w, code, "hosted", map[string]any{
		"Project": p, "Count": count, "Error": errMsg, "Email": email, "Ref": ref,
		"FormTime": s.app.signedFormTime(), "Action": "/v1/w/" + p.PublicKey + "/signup",
		"PageURL": s.app.cfg.BaseURL + "/w/" + p.Slug,
	})
}

func (s *Server) thanksPage(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.ProjectBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, 200, "message", map[string]any{
		"Project": p, "Title": "Check your inbox",
		"Message": "We've sent you a link. Open it to confirm your spot and see your place in line.",
		"Hint":    "Nothing there after a minute? Check your spam folder, and mark the email as not spam so the next one lands.",
	})
}

func (s *Server) statusPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	su, p, err := s.app.SignupByStatusToken(ctx, r.PathValue("token"))
	if err != nil {
		s.render(w, 404, "message", map[string]any{"Title": "Link not recognised", "Message": "This link is invalid or the spot it pointed to was removed."})
		return
	}
	moved := false
	if r.URL.Query().Get("confirm") == "1" && su.Status == StatusPending {
		su, moved, err = s.app.Confirm(ctx, p, su.ID)
		if err != nil {
			s.app.log.Error("confirm", "err", err)
			http.Error(w, "error", 500)
			return
		}
	}
	s.renderStatus(w, r, p, su, moved)
}

// statusConfirm is the button-press fallback for a pending signup that opened
// a status link without ?confirm=1.
func (s *Server) statusConfirm(w http.ResponseWriter, r *http.Request) {
	su, p, err := s.app.SignupByStatusToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, _, err := s.app.Confirm(r.Context(), p, su.ID); err != nil {
		http.Error(w, "error", 500)
		return
	}
	http.Redirect(w, r, "/s/"+r.PathValue("token"), http.StatusSeeOther)
}

func (s *Server) renderStatus(w http.ResponseWriter, r *http.Request, p *Project, su *Signup, justConfirmed bool) {
	pl, err := s.app.PlaceOf(r.Context(), p, su)
	if err != nil {
		http.Error(w, "error", 500)
		return
	}
	data := map[string]any{
		"Project": p, "Signup": su, "Place": pl, "JustConfirmed": justConfirmed,
		"ReferralLink": s.app.ReferralLink(p, su), "Token": s.app.StatusToken(su),
		"ShareText": "I just joined the " + p.Name + " waitlist. Get in line:",
	}
	if su.Status == StatusInvited || su.Status == StatusJoined {
		if inv, err := s.app.LatestInvite(r.Context(), su.ID); err == nil {
			data["Invite"] = inv
			data["InviteLink"] = s.app.InviteLink(p, su, inv.ID)
		}
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, 200, "status", data)
}

func (s *Server) unsubscribePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, 200, "unsubscribe", map[string]any{"Token": r.PathValue("token")})
}

// unsubscribePost serves both the confirm button and RFC 8058 one-click unsubscribe.
func (s *Server) unsubscribePost(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Unsubscribe(r.Context(), r.PathValue("token")); err != nil {
		http.Error(w, "invalid link", http.StatusNotFound)
		return
	}
	if r.FormValue("List-Unsubscribe") == "One-Click" {
		w.WriteHeader(http.StatusOK)
		return
	}
	s.render(w, 200, "message", map[string]any{"Title": "You're unsubscribed", "Message": "You won't get any more updates. Your place in line is unchanged."})
}

// resendHook takes Resend's bounce and complaint events (Svix-signed, which is
// Standard Webhooks) and suppresses those addresses everywhere.
func (s *Server) resendHook(w http.ResponseWriter, r *http.Request) {
	if s.app.cfg.ResendHook == "" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	id, ts, sig := r.Header.Get("svix-id"), r.Header.Get("svix-timestamp"), r.Header.Get("svix-signature")
	if id == "" {
		id, ts, sig = r.Header.Get("webhook-id"), r.Header.Get("webhook-timestamp"), r.Header.Get("webhook-signature")
	}
	if !VerifyWebhook(s.app.cfg.ResendHook, id, ts, sig, body) {
		http.Error(w, "bad signature", 401)
		return
	}
	var ev struct {
		Type string `json:"type"`
		Data struct {
			To []string `json:"to"`
		} `json:"data"`
	}
	json.Unmarshal(body, &ev)
	reason := map[string]string{"email.bounced": "bounce", "email.complained": "complaint"}[ev.Type]
	if reason != "" {
		for _, to := range ev.Data.To {
			if err := s.app.Suppress(r.Context(), to, reason); err != nil {
				http.Error(w, "error", 500)
				return
			}
		}
	}
	w.WriteHeader(204)
}

func (s *Server) widgetJS(w http.ResponseWriter, r *http.Request) {
	b, err := staticFS.ReadFile("web/static/wl.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(b)
}
