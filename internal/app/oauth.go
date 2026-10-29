package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Sign in with Google or GitHub. The flow is plain OAuth 2.0 authorization
// code with a state cookie (and PKCE for Google). An identity is matched by
// the provider's user ID first, then by verified email, so someone who signed
// up with a password can later use "Continue with Google" on the same account.

type OAuthProvider struct {
	ID, Name      string
	ClientID      string
	ClientSecret  string
	AuthURL       string
	TokenURL      string
	Scope         string
	PKCE          bool
	fetchIdentity func(ctx context.Context, token string) (*oauthIdentity, error)
}

type oauthIdentity struct {
	Subject string
	Email   string // verified by the provider
}

var oauthClient = &http.Client{Timeout: 15 * time.Second}

func oauthProviders(c Config) map[string]*OAuthProvider {
	out := map[string]*OAuthProvider{}
	if c.GoogleClientID != "" && c.GoogleClientSecret != "" {
		out["google"] = &OAuthProvider{
			ID: "google", Name: "Google", ClientID: c.GoogleClientID, ClientSecret: c.GoogleClientSecret,
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token",
			Scope: "openid email", PKCE: true, fetchIdentity: googleIdentity,
		}
	}
	if c.GitHubClientID != "" && c.GitHubClientSecret != "" {
		out["github"] = &OAuthProvider{
			ID: "github", Name: "GitHub", ClientID: c.GitHubClientID, ClientSecret: c.GitHubClientSecret,
			AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token",
			Scope: "read:user user:email", fetchIdentity: githubIdentity,
		}
	}
	return out
}

func getJSON(ctx context.Context, u, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := oauthClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

func googleIdentity(ctx context.Context, token string) (*oauthIdentity, error) {
	var u struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := getJSON(ctx, "https://openidconnect.googleapis.com/v1/userinfo", token, &u); err != nil {
		return nil, err
	}
	if u.Sub == "" || u.Email == "" || !u.EmailVerified {
		return nil, errors.New("Google didn't share a verified email address.")
	}
	return &oauthIdentity{Subject: u.Sub, Email: u.Email}, nil
}

func githubIdentity(ctx context.Context, token string) (*oauthIdentity, error) {
	var u struct {
		ID int64 `json:"id"`
	}
	if err := getJSON(ctx, "https://api.github.com/user", token, &u); err != nil {
		return nil, err
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := getJSON(ctx, "https://api.github.com/user/emails", token, &emails); err != nil {
		return nil, err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return &oauthIdentity{Subject: strconv.FormatInt(u.ID, 10), Email: e.Email}, nil
		}
	}
	return nil, errors.New("Your GitHub account has no verified primary email.")
}

// exchange trades the authorization code for an access token.
func (p *OAuthProvider) exchange(ctx context.Context, code, redirectURI, verifier string) (string, error) {
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI},
		"client_id": {p.ClientID}, "client_secret": {p.ClientSecret},
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var t struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t)
	if t.AccessToken == "" {
		return "", fmt.Errorf("%s token exchange failed: HTTP %d %s", p.Name, resp.StatusCode, t.Error)
	}
	return t.AccessToken, nil
}

// OAuthAccount finds or creates the account for a provider identity.
// Self-hosted instances never create accounts this way: the identity must
// match the existing admin's email.
func (a *App) OAuthAccount(ctx context.Context, provider string, id *oauthIdentity) (int64, bool, error) {
	var adminID int64
	err := a.db.W.QueryRowContext(ctx, `SELECT admin_id FROM oauth_identities WHERE provider = ? AND subject = ?`, provider, id.Subject).Scan(&adminID)
	if err == nil {
		return adminID, false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	created := false
	err = a.db.W.QueryRowContext(ctx, `SELECT id FROM admins WHERE email = ? COLLATE NOCASE`, id.Email).Scan(&adminID)
	if errors.Is(err, sql.ErrNoRows) {
		if !a.cfg.Cloud {
			return 0, false, errors.New("No account here uses that email address.")
		}
		res, err := a.db.W.ExecContext(ctx, `INSERT INTO admins (email, password_hash, created_at, verified_at) VALUES (?, '', ?, ?)`, id.Email, now(), now())
		if err != nil {
			return 0, false, err
		}
		adminID, _ = res.LastInsertId()
		created = true
	} else if err != nil {
		return 0, false, err
	} else {
		// Someone may have registered this address with a password without
		// owning it. The provider proves who does, so an unconfirmed account
		// loses that password and its sessions before it's linked.
		a.db.W.ExecContext(ctx, `UPDATE admins SET password_hash = '' WHERE id = ? AND verified_at IS NULL`, adminID)
		a.db.W.ExecContext(ctx, `DELETE FROM sessions WHERE admin_id = ? AND (SELECT verified_at FROM admins WHERE id = ?) IS NULL`, adminID, adminID)
	}
	// The provider vouched for the address, so it counts as confirmed.
	a.db.W.ExecContext(ctx, `UPDATE admins SET verified_at = COALESCE(verified_at, ?) WHERE id = ?`, now(), adminID)
	_, err = a.db.W.ExecContext(ctx, `INSERT OR IGNORE INTO oauth_identities (provider, subject, admin_id, email, created_at) VALUES (?, ?, ?, ?, ?)`,
		provider, id.Subject, adminID, id.Email, now())
	return adminID, created, err
}

const oauthCookie = "lobby_oauth"

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (s *Server) oauthRedirectURI(p *OAuthProvider) string {
	return s.app.cfg.BaseURL + "/auth/" + p.ID + "/callback"
}

// safePath keeps post-login redirects on this site.
func safePath(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.HasPrefix(next, "/\\") && next != "/" {
		return next
	}
	return "/dashboard"
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	p := s.app.oauth[r.PathValue("provider")]
	if p == nil {
		http.NotFound(w, r)
		return
	}
	state, verifier := randHex(16), ""
	if p.PKCE {
		verifier = randHex(32)
	}
	next := safePath(r.URL.Query().Get("next"))
	// state|verifier|next, readable only by us: the cookie is HttpOnly and the
	// state is compared in constant time on the way back.
	http.SetCookie(w, &http.Cookie{
		Name: oauthCookie, Value: base64.RawURLEncoding.EncodeToString([]byte(state + "|" + verifier + "|" + next)),
		Path: "/auth/", MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: strings.HasPrefix(s.app.cfg.BaseURL, "https://"),
	})
	q := url.Values{
		"client_id": {p.ClientID}, "redirect_uri": {s.oauthRedirectURI(p)}, "response_type": {"code"},
		"scope": {p.Scope}, "state": {state},
	}
	if p.PKCE {
		q.Set("code_challenge", pkceChallenge(verifier))
		q.Set("code_challenge_method", "S256")
	}
	if p.ID == "google" {
		q.Set("prompt", "select_account")
	}
	http.Redirect(w, r, p.AuthURL+"?"+q.Encode(), http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	p := s.app.oauth[r.PathValue("provider")]
	if p == nil {
		http.NotFound(w, r)
		return
	}
	fail := func(msg string) {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(msg), http.StatusFound)
	}
	c, err := r.Cookie(oauthCookie)
	http.SetCookie(w, &http.Cookie{Name: oauthCookie, Path: "/auth/", MaxAge: -1})
	if err != nil {
		fail("That sign-in link expired. Please try again.")
		return
	}
	raw, _ := base64.RawURLEncoding.DecodeString(c.Value)
	parts := strings.SplitN(string(raw), "|", 3)
	q := r.URL.Query()
	if len(parts) != 3 || q.Get("state") == "" || subtle.ConstantTimeCompare([]byte(parts[0]), []byte(q.Get("state"))) != 1 {
		fail("That sign-in link expired. Please try again.")
		return
	}
	if q.Get("error") != "" || q.Get("code") == "" {
		fail(p.Name + " sign-in was cancelled.")
		return
	}
	if !s.app.spam.Allow("oauth:"+s.clientIP(r), 20, 15*time.Minute) {
		fail("Too many attempts. Wait a few minutes.")
		return
	}
	ctx := r.Context()
	token, err := p.exchange(ctx, q.Get("code"), s.oauthRedirectURI(p), parts[1])
	if err != nil {
		s.app.log.Error("oauth exchange", "provider", p.ID, "err", err)
		fail("Couldn't sign in with " + p.Name + ". Please try again.")
		return
	}
	id, err := p.fetchIdentity(ctx, token)
	if err != nil {
		s.app.log.Error("oauth identity", "provider", p.ID, "err", err)
		fail(err.Error())
		return
	}
	adminID, created, err := s.app.OAuthAccount(ctx, p.ID, id)
	if err != nil {
		fail(err.Error())
		return
	}
	tok, err := s.app.newSession(ctx, adminID)
	if err != nil {
		fail("Something went wrong. Please try again.")
		return
	}
	s.setCookie(w, tok, sessionTTL)
	if created {
		s.app.log.Info("account created", "via", p.ID, "account", adminID)
	}
	http.Redirect(w, r, safePath(parts[2]), http.StatusFound)
}
