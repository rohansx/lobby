package app

import (
	"net/http"
	"time"
)

// Cloud account and billing endpoints. They 404 on a self-hosted instance.

func (s *Server) cloudOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.app.cfg.Cloud {
			jsonError(w, http.StatusNotFound, "not found")
			return
		}
		next(w, r)
	}
}

func (s *Server) adminSignup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if !s.app.spam.Allow("signup:"+s.clientIP(r), 5, time.Hour) {
		jsonError(w, 429, "Too many accounts from this network. Try again later.")
		return
	}
	id, err := s.app.SignUp(r.Context(), c.Email, c.Password)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	if ac, err := s.app.Account(r.Context(), id); err == nil {
		if err := s.app.SendVerification(r.Context(), ac); err != nil {
			s.app.log.Error("verification email", "err", err)
		}
	}
	s.startSession(w, r, c)
}

func (s *Server) adminResendVerification(w http.ResponseWriter, r *http.Request) {
	ac, err := s.app.Account(r.Context(), adminID(r))
	if err != nil || ac.Verified {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if !s.app.spam.Allow("verify:"+ac.Email, 3, time.Hour) {
		jsonError(w, 429, "We've sent a few already. Check your spam folder, or try again in an hour.")
		return
	}
	if err := s.app.SendVerification(r.Context(), ac); err != nil {
		jsonError(w, 500, "Couldn't send the email. Try again shortly.")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// verifyPage is the link in the verification email.
func (s *Server) verifyPage(w http.ResponseWriter, r *http.Request) {
	if !s.app.cfg.Cloud {
		http.NotFound(w, r)
		return
	}
	if _, err := s.app.Verify(r.Context(), r.PathValue("token")); err != nil {
		s.render(w, 400, "message", map[string]any{"Title": "Link expired", "Message": err.Error() + " Sign in and send a new one from the dashboard.", "Back": "/"})
		return
	}
	http.Redirect(w, r, "/dashboard?verified=1", http.StatusSeeOther)
}

func (s *Server) adminForgot(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email string }
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	if !s.app.spam.Allow("forgot:"+s.clientIP(r), 5, time.Hour) {
		jsonError(w, 429, "Too many attempts. Try again later.")
		return
	}
	if err := s.app.SendReset(r.Context(), body.Email); err != nil {
		s.app.log.Error("reset email", "err", err)
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) adminReset(w http.ResponseWriter, r *http.Request) {
	var body struct{ Token, Password string }
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	email, err := s.app.ResetPassword(r.Context(), body.Token, body.Password)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	s.startSession(w, r, credentials{Email: email, Password: body.Password})
}

func (s *Server) adminBilling(w http.ResponseWriter, r *http.Request) {
	ac, err := s.app.Account(r.Context(), adminID(r))
	if err != nil {
		jsonError(w, 404, "not found")
		return
	}
	list := []Plan{plans["free"], plans["launch"], plans["pro"]}
	writeJSON(w, 200, map[string]any{
		"account": ac, "plan": s.app.PlanOf(ac), "usage": s.app.UsageOf(r.Context(), ac.ID),
		"plans": list, "billing_enabled": s.app.cfg.Polar.Enabled(),
	})
}

func (s *Server) adminCheckout(w http.ResponseWriter, r *http.Request) {
	var body struct{ Plan, Interval string }
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	ac, err := s.app.Account(r.Context(), adminID(r))
	if err != nil {
		jsonError(w, 404, "not found")
		return
	}
	if !ac.Verified {
		jsonError(w, 400, "Confirm your email address first.")
		return
	}
	u, err := s.app.Checkout(r.Context(), ac, body.Plan, body.Interval)
	if err != nil {
		s.app.log.Error("checkout", "err", err)
		jsonError(w, 400, userMessage(err))
		return
	}
	writeJSON(w, 200, map[string]any{"url": u})
}

func (s *Server) adminPortal(w http.ResponseWriter, r *http.Request) {
	ac, err := s.app.Account(r.Context(), adminID(r))
	if err != nil {
		jsonError(w, 404, "not found")
		return
	}
	u, err := s.app.Portal(r.Context(), ac)
	if err != nil {
		s.app.log.Error("portal", "err", err)
		jsonError(w, 400, userMessage(err))
		return
	}
	writeJSON(w, 200, map[string]any{"url": u})
}

func (s *Server) adminDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var body struct{ Password string }
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	ac, err := s.app.Account(r.Context(), adminID(r))
	if err != nil {
		jsonError(w, 404, "not found")
		return
	}
	if ac.PolarSubscription != "" && paidStatus(ac.SubStatus) && !ac.CancelAtPeriodEnd {
		jsonError(w, 400, "Cancel your subscription from Manage billing first, so you aren't charged again.")
		return
	}
	if err := s.app.DeleteAccount(r.Context(), ac.ID, body.Password); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	s.setCookie(w, "", -1)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// userMessage keeps provider error bodies out of the dashboard.
func userMessage(err error) string {
	switch err.Error() {
	case errBillingOff.Error(), "unknown plan", "You already have a subscription. Change plans from Manage billing.":
		return err.Error()
	}
	return "Billing is unavailable right now. Try again in a minute."
}
