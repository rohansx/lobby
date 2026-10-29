package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeProvider points a provider at a local token endpoint and returns a
// fixed identity, so the whole start → callback flow runs offline.
func fakeProvider(t *testing.T, a *App, id *oauthIdentity) {
	t.Helper()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	}))
	t.Cleanup(tokenSrv.Close)
	a.oauth = map[string]*OAuthProvider{"google": {
		ID: "google", Name: "Google", ClientID: "cid", ClientSecret: "secret",
		AuthURL: "https://accounts.example/auth", TokenURL: tokenSrv.URL, Scope: "openid email", PKCE: true,
		fetchIdentity: func(context.Context, string) (*oauthIdentity, error) { return id, nil },
	}}
}

// oauthLogin runs the flow and returns the final redirect and session cookie.
func oauthLogin(t *testing.T, h http.Handler, state string) (string, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/google/start?next=/billing", nil))
	if rec.Code != 302 {
		t.Fatalf("start: %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != "http://lobby.test/auth/google/callback" {
		t.Fatalf("authorize URL: %s", loc)
	}
	if state == "" {
		state = q.Get("state")
	}
	req := httptest.NewRequest("GET", "/auth/google/callback?code=good-code&state="+state, nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			session = c
		}
	}
	return rec.Header().Get("Location"), session
}

func TestOAuthSignupAndLinking(t *testing.T) {
	a := newCloudApp(t)
	ctx := context.Background()
	srv, _ := NewServer(a)
	h := srv.Handler()

	fakeProvider(t, a, &oauthIdentity{Subject: "g-1", Email: "new@example.com"})
	if loc, s := oauthLogin(t, h, "forged"); s != nil || !strings.HasPrefix(loc, "/login?error=") {
		t.Fatalf("a wrong state must not sign in: %s", loc)
	}
	loc, s := oauthLogin(t, h, "")
	if s == nil || loc != "/billing" {
		t.Fatalf("new account: %s %v", loc, s)
	}
	id, _ := a.SessionAdmin(ctx, s.Value)
	ac, _ := a.Account(ctx, id)
	if ac.Email != "new@example.com" || !ac.Verified || ac.HasPassword {
		t.Fatalf("account: %+v", ac)
	}
	if _, err := a.Login(ctx, "new@example.com", ""); err == nil || !strings.Contains(err.Error(), "Google or GitHub") {
		t.Fatalf("passwordless login: %v", err)
	}
	// Same Google user again lands on the same account.
	_, s2 := oauthLogin(t, h, "")
	if id2, _ := a.SessionAdmin(ctx, s2.Value); id2 != id {
		t.Fatal("returning user should reuse the account")
	}

	// Someone registered victim@example.com with a password but never
	// confirmed it. When the real owner signs in with Google, that password
	// and its sessions must stop working.
	squatter, _ := a.SignUp(ctx, "victim@example.com", "squatter password")
	squatTok, _ := a.newSession(ctx, squatter)
	fakeProvider(t, a, &oauthIdentity{Subject: "g-2", Email: "Victim@example.com"})
	_, s3 := oauthLogin(t, h, "")
	if owner, _ := a.SessionAdmin(ctx, s3.Value); owner != squatter {
		t.Fatal("should link to the existing account")
	}
	if _, ok := a.SessionAdmin(ctx, squatTok); ok {
		t.Fatal("the squatter's session must be revoked")
	}
	if _, err := a.Login(ctx, "victim@example.com", "squatter password"); err == nil {
		t.Fatal("the squatter's password must be cleared")
	}

	// A confirmed account keeps its password when Google is linked.
	owner, _ := a.SignUp(ctx, "owner@example.com", "owner password 1")
	a.db.W.Exec(`UPDATE admins SET verified_at = 1 WHERE id = ?`, owner)
	fakeProvider(t, a, &oauthIdentity{Subject: "g-3", Email: "owner@example.com"})
	oauthLogin(t, h, "")
	if _, err := a.Login(ctx, "owner@example.com", "owner password 1"); err != nil {
		t.Fatalf("verified account keeps its password: %v", err)
	}
}

func TestOAuthSelfHostedNeedsExistingAdmin(t *testing.T) {
	a := newTestApp(t)
	srv, _ := NewServer(a)
	h := srv.Handler()
	a.CreateFirstAdmin(context.Background(), "admin@example.com", "correct horse battery")
	fakeProvider(t, a, &oauthIdentity{Subject: "g-9", Email: "stranger@example.com"})
	if loc, s := oauthLogin(t, h, ""); s != nil || !strings.Contains(loc, "error=") {
		t.Fatalf("strangers can't sign in to a self-hosted instance: %s", loc)
	}
	fakeProvider(t, a, &oauthIdentity{Subject: "g-10", Email: "admin@example.com"})
	if _, s := oauthLogin(t, h, ""); s == nil {
		t.Fatal("the admin can sign in with Google")
	}
}
