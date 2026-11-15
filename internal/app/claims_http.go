package app

import (
	"errors"
	"net/http"
	"time"
)

// ---- API (secret key) ----

func (s *Server) apiCreateClaim(w http.ResponseWriter, r *http.Request) {
	var in ClaimInput
	if err := decodeJSON(r, &in); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	c, err := s.app.CreateClaim(r.Context(), apiProject(r), in)
	if !s.claimErr(w, err) {
		writeJSON(w, 201, s.claimOut(c))
	}
}

func (s *Server) apiListClaims(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.ListClaims(r.Context(), apiProject(r))
	if s.claimErr(w, err) {
		return
	}
	out := make([]map[string]any, len(list))
	for i, c := range list {
		out[i] = s.claimOut(c)
	}
	writeJSON(w, 200, map[string]any{"data": out})
}

func (s *Server) apiRevokeClaim(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		jsonError(w, 400, "bad id")
		return
	}
	if !s.claimErr(w, s.app.RevokeClaim(r.Context(), apiProject(r), id)) {
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

func (s *Server) apiRedeemClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Code == "" {
		jsonError(w, 400, `send {"code": "..."}, the value of ?claim= on your page`)
		return
	}
	p := apiProject(r)
	c, err := s.app.RedeemClaim(r.Context(), p, body.Code)
	if s.claimErr(w, err) {
		return
	}
	out := map[string]any{"account_ref": c.AccountRef, "email": c.Email, "label": c.Label, "claim_id": c.ID, "signup_id": c.SignupID}
	if c.SignupID != nil {
		if su, err := getSignup(r.Context(), s.app.db.R, p.ID, *c.SignupID); err == nil {
			out["reporter_token"] = s.app.ReporterToken(su)
		}
	}
	writeJSON(w, 200, out)
}

// claimOut is a claim as the API and dashboard return it.
func (s *Server) claimOut(c *Claim) map[string]any {
	return map[string]any{"id": c.ID, "account_ref": c.AccountRef, "label": c.Label, "email_lock": c.EmailLock, "email": c.Email, "pending_email": c.Pending,
		"signup_id": c.SignupID, "state": c.State(), "claim_url": s.app.ClaimURL(c), "created_at": c.CreatedAt,
		"expires_at": c.ExpiresAt, "opened_at": c.OpenedAt, "verified_at": c.VerifiedAt, "claimed_at": c.ClaimedAt}
}

// claimErr writes the error response, if any, and reports whether it did.
func (s *Server) claimErr(w http.ResponseWriter, err error) bool {
	var ve *ValidationError
	switch {
	case err == nil:
		return false
	case errors.As(err, &ve):
		jsonError(w, 400, ve.Msg)
	case errors.Is(err, ErrInvalidClaim):
		jsonError(w, 404, err.Error())
	case errors.Is(err, ErrNotFound):
		jsonError(w, 404, "no such claim")
	default:
		s.app.log.Error("claims", "err", err)
		jsonError(w, 500, "something went wrong")
	}
	return true
}

// ---- dashboard (every plan) ----

func (s *Server) adminClaims(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	list, err := s.app.ListClaims(r.Context(), p)
	if s.claimErr(w, err) {
		return
	}
	out := make([]map[string]any, len(list))
	for i, c := range list {
		out[i] = s.claimOut(c)
	}
	writeJSON(w, 200, map[string]any{"data": out, "target": claimTarget(p)})
}

func (s *Server) adminCreateClaim(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	var in ClaimInput
	if err := decodeJSON(r, &in); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	c, err := s.app.CreateClaim(r.Context(), p, in)
	if !s.claimErr(w, err) {
		writeJSON(w, 201, s.claimOut(c))
	}
}

func (s *Server) adminRevokeClaim(w http.ResponseWriter, r *http.Request) {
	p := s.adminProject(w, r)
	if p == nil {
		return
	}
	id, ok := pathID(r)
	if !ok {
		jsonError(w, 400, "bad id")
		return
	}
	if !s.claimErr(w, s.app.RevokeClaim(r.Context(), p, id)) {
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

// ---- public pages ----

func (s *Server) claimPage(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.liveClaim(w, r)
	if !ok {
		return
	}
	s.app.MarkClaimOpened(r.Context(), p, c)
	s.render(w, 200, "claim", map[string]any{"Project": p, "Claim": c, "Email": c.EmailLock})
}

func (s *Server) claimPost(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.liveClaim(w, r)
	if !ok {
		return
	}
	if !s.app.spam.Allow("claim-ip:"+s.clientIP(r), 20, time.Hour) {
		s.render(w, 429, "claim", map[string]any{"Project": p, "Claim": c, "Email": c.EmailLock, "Error": "Too many tries. Please wait a few minutes."})
		return
	}
	email := r.PostFormValue("email")
	if c.EmailLock != "" {
		email = c.EmailLock
	}
	_, err := s.app.StartClaim(r.Context(), p, c, email)
	var ve *ValidationError
	switch {
	case errors.As(err, &ve):
		s.render(w, 400, "claim", map[string]any{"Project": p, "Claim": c, "Email": email, "Error": ve.Msg})
	case err != nil:
		s.claimGone(w, p, err)
	default:
		s.render(w, 200, "message", map[string]any{"Project": p, "Title": "Check your inbox",
			"Message": "We sent a link to " + email + ". Open it to claim your account.",
			"Hint":    "Nothing there after a minute? Check your spam folder."})
	}
}

// claimConfirmPage is what the email link opens: one button. Issuing the
// code on GET would let link-scanning mail filters spend it.
func (s *Server) claimConfirmPage(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.liveClaim(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, 200, "claim", map[string]any{"Project": p, "Claim": c, "Confirm": true, "Email": c.Pending})
}

func (s *Server) claimConfirm(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.liveClaim(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	target, err := s.app.ConfirmClaim(r.Context(), p, c.ID, r.PathValue("sig"))
	if err != nil {
		s.claimGone(w, p, err)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) liveClaim(w http.ResponseWriter, r *http.Request) (*Project, *Claim, bool) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	p, c, err := s.app.ClaimByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		s.render(w, 404, "message", map[string]any{"Title": "Link not recognised", "Message": "This claim link is invalid. Check you copied all of it."})
		return nil, nil, false
	}
	if !c.live() {
		s.claimGone(w, p, ErrInvalidClaim)
		return nil, nil, false
	}
	return p, c, true
}

func (s *Server) claimGone(w http.ResponseWriter, p *Project, err error) {
	if !errors.Is(err, ErrInvalidClaim) {
		s.app.log.Error("claim page", "err", err)
		s.render(w, 500, "message", map[string]any{"Project": p, "Title": "Something went wrong", "Message": "Please try the link again in a minute."})
		return
	}
	s.render(w, 410, "message", map[string]any{"Project": p, "Title": "This link is no longer active",
		"Message": "It was used, revoked or has expired. Ask " + p.Name + " for a new one."})
}
