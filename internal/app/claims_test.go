package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func claimProject(t *testing.T, a *App) *Project {
	t.Helper()
	p := newTestProject(t, a, true)
	p.InviteURL = "https://app.example.com/welcome"
	if err := a.UpdateProject(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

// confirmClaim walks the prospect's half: enter an email, then open the
// confirmation link. It returns the redeem code from the redirect.
func confirmClaim(t *testing.T, a *App, p *Project, c *Claim, email string) string {
	t.Helper()
	ctx := context.Background()
	link, err := a.StartClaim(ctx, p, c, email)
	if err != nil {
		t.Fatalf("start claim: %v", err)
	}
	sig := link[strings.LastIndex(link, "/")+1:]
	target, err := a.ConfirmClaim(ctx, p, c.ID, sig)
	if err != nil {
		t.Fatalf("confirm claim: %v", err)
	}
	u, _ := url.Parse(target)
	if !strings.HasPrefix(target, p.InviteURL) || u.Query().Get("claim") == "" {
		t.Fatalf("redirect should go to the invite URL with ?claim=, got %s", target)
	}
	return u.Query().Get("claim")
}

func TestClaimHappyPath(t *testing.T) {
	a := newTestApp(t)
	p := claimProject(t, a)
	ctx := context.Background()

	c, err := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_123", Label: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.ClaimURL(c), "http://lobby.test/c/") {
		t.Fatalf("claim URL: %s", a.ClaimURL(c))
	}
	code := confirmClaim(t, a, p, c, "Priya@Acme.com")

	got, err := a.RedeemClaim(ctx, p, code)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccountRef != "acct_123" || got.Email != "Priya@Acme.com" || got.SignupID == nil {
		t.Fatalf("redeem returned %+v", got)
	}
	su, _ := getSignup(ctx, a.db.W, p.ID, *got.SignupID)
	if su.Status != StatusJoined || su.UTMSource != claimSource {
		t.Fatalf("claimed prospect should be a joined signup from %q, got %s/%s", claimSource, su.Status, su.UTMSource)
	}
	if _, err := a.RedeemClaim(ctx, p, code); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("a code works once, got %v", err)
	}
}

func TestClaimEmailLock(t *testing.T) {
	a := newTestApp(t)
	p := claimProject(t, a)
	ctx := context.Background()
	c, err := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_1", Email: "priya@acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	var ve *ValidationError
	if _, err := a.StartClaim(ctx, p, c, "someone@else.com"); !errors.As(err, &ve) {
		t.Fatalf("a locked claim should refuse another address, got %v", err)
	}
	code := confirmClaim(t, a, p, c, "Priya+x@acme.com") // same address after normalizing
	got, err := a.RedeemClaim(ctx, p, code)
	if err != nil || got.Email != "priya@acme.com" {
		t.Fatalf("a locked claim hands over the locked address, got %+v %v", got, err)
	}
}

// Someone else holding an unlocked link can't swap their unverified address
// in after the prospect confirmed: typing a new address voids the code.
func TestClaimCodeBoundToVerifiedEmail(t *testing.T) {
	a := newTestApp(t)
	p := claimProject(t, a)
	ctx := context.Background()
	c, _ := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_1"})
	code := confirmClaim(t, a, p, c, "priya@acme.com")
	if _, err := a.StartClaim(ctx, p, c, "intruder@evil.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RedeemClaim(ctx, p, code); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("the code should die when the address changes, got %v", err)
	}
	code = confirmClaim(t, a, p, c, "priya@acme.com")
	got, err := a.RedeemClaim(ctx, p, code)
	if err != nil || got.Email != "priya@acme.com" {
		t.Fatalf("want priya after re-confirming, got %+v %v", got, err)
	}
}

func TestClaimStates(t *testing.T) {
	a := newTestApp(t)
	p := claimProject(t, a)
	ctx := context.Background()

	first, _ := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_9"})
	second, _ := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_9"})
	if c, _ := a.ClaimByID(ctx, p, first.ID); c.State() != "revoked" {
		t.Fatalf("a new claim for the same account revokes the old one, got %s", c.State())
	}
	if _, err := a.StartClaim(ctx, p, first, "x@acme.com"); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("revoked claim should refuse, got %v", err)
	}

	if err := a.RevokeClaim(ctx, p, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartClaim(ctx, p, second, "x@acme.com"); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("revoked claim should refuse, got %v", err)
	}

	expired, _ := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_old", ExpiresIn: 60})
	a.db.W.Exec(`UPDATE claims SET expires_at = ? WHERE id = ?`, now()-1, expired.ID)
	expired, _ = a.ClaimByID(ctx, p, expired.ID)
	if expired.State() != "expired" {
		t.Fatalf("want expired, got %s", expired.State())
	}

	c, _ := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_code"})
	code := confirmClaim(t, a, p, c, "late@acme.com")
	a.db.W.Exec(`UPDATE claims SET code_expires_at = ? WHERE id = ?`, now()-1, c.ID)
	if _, err := a.RedeemClaim(ctx, p, code); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("an expired code should refuse, got %v", err)
	}
}

func TestClaimValidation(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	p := newTestProject(t, a, true) // no invite URL or site URL
	var ve *ValidationError
	if _, err := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_1"}); !errors.As(err, &ve) {
		t.Fatalf("a claim needs somewhere to send people, got %v", err)
	}
	p = claimProject(t, a)
	for _, in := range []ClaimInput{{}, {AccountRef: strings.Repeat("x", 300)}, {AccountRef: "a", Email: "nope"}, {AccountRef: "a", ExpiresIn: -5}} {
		if _, err := a.CreateClaim(ctx, p, in); !errors.As(err, &ve) {
			t.Fatalf("%+v should be refused, got %v", in, err)
		}
	}
	c, _ := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_1"})
	if _, err := a.StartClaim(ctx, p, c, "x@mailinator.com"); !errors.As(err, &ve) {
		t.Fatalf("disposable address should be refused, got %v", err)
	}
	if _, err := a.ConfirmClaim(ctx, p, c.ID, "forged"); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("a forged confirmation should refuse, got %v", err)
	}
}

func TestClaimAttachesToExistingSignup(t *testing.T) {
	a := newTestApp(t)
	p := claimProject(t, a)
	ctx := context.Background()
	existing := signup(t, a, p, "priya@acme.com", "", "1.1.1.1").Signup
	c, _ := a.CreateClaim(ctx, p, ClaimInput{AccountRef: "acct_1"})
	got, err := a.RedeemClaim(ctx, p, confirmClaim(t, a, p, c, "priya@acme.com"))
	if err != nil {
		t.Fatal(err)
	}
	if got.SignupID == nil || *got.SignupID != existing.ID {
		t.Fatalf("claim should attach to the signup already on the list")
	}
	su, _ := getSignup(ctx, a.db.W, p.ID, existing.ID)
	if su.Status != StatusJoined {
		t.Fatalf("existing signup should be joined, got %s", su.Status)
	}
}

func TestClaimOverHTTP(t *testing.T) {
	a := newTestApp(t)
	a.mailer = &captureMailer{}
	p := claimProject(t, a)
	sk, err := a.RotateSecret(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := NewServer(a)
	h := srv.Handler()
	do := func(method, path, body string, key bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if key {
			r.Header.Set("Authorization", "Bearer "+sk)
			r.Header.Set("Content-Type", "application/json")
		} else if body != "" {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := do("POST", "/v1/claims", `{"account_ref": "acct_42", "label": "Acme"}`, true)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	claimURL := regexp.MustCompile(`"claim_url": "([^"]+)"`).FindStringSubmatch(w.Body.String())[1]
	path := strings.TrimPrefix(claimURL, "http://lobby.test")

	if w := do("GET", path, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "Convox") {
		t.Fatalf("claim page: %d", w.Code)
	}
	if w := do("POST", path, "email=priya%40acme.com", false); w.Code != 200 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	a.runJobs(context.Background())
	sent := a.mailer.(*captureMailer).snapshot()
	if len(sent) != 1 || sent[0].To != "priya@acme.com" {
		t.Fatalf("want one confirmation email to priya, got %d", len(sent))
	}
	confirm := regexp.MustCompile(`http://lobby.test(/c/\S+/confirm/\S+)`).FindStringSubmatch(sent[0].Text)[1]
	if w := do("GET", confirm, "", false); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("the email link opens a page with a button, got %d", w.Code)
	}
	w = do("POST", confirm, "", false)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("confirm should redirect, got %d", w.Code)
	}
	code, _ := url.Parse(w.Header().Get("Location"))

	w = do("POST", "/v1/claims/redeem", `{"code": "`+code.Query().Get("claim")+`"}`, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"account_ref": "acct_42"`) || !strings.Contains(w.Body.String(), `"email": "priya@acme.com"`) {
		t.Fatalf("redeem: %d %s", w.Code, w.Body)
	}
	if w := do("GET", "/v1/claims", "", true); !strings.Contains(w.Body.String(), `"state": "claimed"`) {
		t.Fatalf("list should show the claim as claimed: %s", w.Body)
	}
	if w := do("GET", path, "", false); w.Code != 410 {
		t.Fatalf("a claimed link should be gone, got %d", w.Code)
	}
}
