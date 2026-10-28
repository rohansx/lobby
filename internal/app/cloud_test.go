package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newCloudApp(t *testing.T) *App {
	t.Helper()
	cfg := Config{BaseURL: "http://lobby.test", DataDir: t.TempDir(), Cloud: true, Polar: PolarConfig{
		WebhookSecret: "whsec_dGVzdC1zZWNyZXQtZm9yLXBvbGFy",
		Products:      map[string]string{"launch_month": "prod-launch-m", "pro_year": "prod-pro-y"},
	}}
	a, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.mailer = &captureMailer{}
	t.Cleanup(a.Close)
	return a
}

// client is a cookie-holding caller against the handler.
type client struct {
	t      *testing.T
	h      http.Handler
	cookie *http.Cookie
}

func (c *client) do(method, path, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookie {
			c.cookie = ck
		}
	}
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func verifyAll(a *App) {
	a.db.W.Exec(`UPDATE admins SET verified_at = 1`)
}

func TestCloudAccountsAreIsolated(t *testing.T) {
	a := newCloudApp(t)
	srv, _ := NewServer(a)
	h := srv.Handler()
	alice, bob := &client{t: t, h: h}, &client{t: t, h: h}

	if code, me := alice.do("GET", "/api/admin/me", ""); code != 200 || me["setup"] != false || me["cloud"] != true {
		t.Fatalf("cloud never shows setup: %v", me)
	}
	if code, _ := alice.do("POST", "/api/admin/setup", `{"email":"x@example.com","password":"correct horse battery"}`); code != 404 {
		t.Fatalf("setup must be off in cloud: %d", code)
	}
	if code, out := alice.do("POST", "/api/admin/signup", `{"email":"alice@example.com","password":"correct horse battery"}`); code != 200 {
		t.Fatalf("signup: %d %v", code, out)
	}
	if code, _ := bob.do("POST", "/api/admin/signup", `{"email":"ALICE@example.com","password":"correct horse battery"}`); code != 400 {
		t.Fatalf("duplicate email must fail: %d", code)
	}
	bob.do("POST", "/api/admin/signup", `{"email":"bob@example.com","password":"correct horse battery"}`)

	// Unverified accounts can't create waitlists.
	if code, out := alice.do("POST", "/api/admin/projects", `{"name":"Orbit","slug":"orbit"}`); code != 400 || !strings.Contains(out["error"].(string), "Confirm") {
		t.Fatalf("unverified create: %d %v", code, out)
	}
	mc := a.mailer.(*captureMailer)
	var link string
	for _, m := range mc.sent {
		if m.To == "alice@example.com" && strings.Contains(m.Text, "/verify/") {
			i := strings.Index(m.Text, "http://lobby.test/verify/")
			link = strings.Fields(m.Text[i:])[0]
		}
	}
	if link == "" {
		t.Fatal("no verification email")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", strings.TrimPrefix(link, "http://lobby.test"), nil))
	if rec.Code != 303 {
		t.Fatalf("verify link: %d", rec.Code)
	}
	if _, me := alice.do("GET", "/api/admin/me", ""); me["verified"] != true {
		t.Fatal("alice should be verified")
	}
	verifyAll(a)

	if code, _ := alice.do("POST", "/api/admin/projects", `{"name":"Orbit","slug":"orbit"}`); code != 201 {
		t.Fatalf("alice create: %d", code)
	}
	if code, _ := bob.do("GET", "/api/admin/projects/orbit", ""); code != 404 {
		t.Fatalf("bob must not see alice's project: %d", code)
	}
	if code, _ := bob.do("PATCH", "/api/admin/projects/orbit", `{"name":"Mine"}`); code != 404 {
		t.Fatalf("bob must not edit alice's project: %d", code)
	}
	if _, out := bob.do("GET", "/api/admin/projects", ""); len(out["data"].([]any)) != 0 {
		t.Fatalf("bob's list should be empty: %v", out["data"])
	}
	if code, _ := bob.do("POST", "/api/admin/projects", `{"name":"Lobby","slug":"lobby"}`); code != 400 {
		t.Fatalf("reserved slug: %d", code)
	}

	// Free plan: one waitlist, no webhooks, no API.
	if code, out := alice.do("POST", "/api/admin/projects", `{"name":"Two","slug":"two"}`); code != 402 || out["upgrade"] != true {
		t.Fatalf("second waitlist on free: %d %v", code, out)
	}
	if code, _ := alice.do("POST", "/api/admin/projects/orbit/webhooks", `{"url":"https://example.com/hook"}`); code != 402 {
		t.Fatalf("webhook on free: %d", code)
	}
}

func TestCloudSignupLimit(t *testing.T) {
	a := newCloudApp(t)
	ctx := context.Background()
	id, err := a.SignUp(ctx, "owner@example.com", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := a.CreateProject(ctx, id, "Orbit", "orbit")
	if err != nil {
		t.Fatal(err)
	}
	a.db.W.Exec(`INSERT INTO signups (project_id, email, email_norm, status, base_position, referral_code, created_at)
		WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 499)
		SELECT ?, 'u'||i||'@example.com', 'u'||i||'@example.com', 'confirmed', i, 'code'||i, 0 FROM n`, p.ID)
	signup(t, a, p, "last@example.com", "", "1.1.1.1") // the 500th
	_, err = a.SubmitSignup(ctx, p, SignupInput{Email: "over@example.com", DwellMs: -1})
	if err != ErrWaitlistFull {
		t.Fatalf("501st signup should be refused, got %v", err)
	}
	// Someone already on the list still gets their link back.
	if res := signup(t, a, p, "last@example.com", "", "1.1.1.1"); res.Created || res.Signup == nil {
		t.Fatal("existing signup should be found")
	}
	time.Sleep(50 * time.Millisecond) // the limit email goes out in the background
	found := false
	for _, m := range a.mailer.(*captureMailer).sent {
		found = found || (m.To == "owner@example.com" && strings.Contains(m.Subject, "full"))
	}
	if !found {
		t.Fatal("owner should hear the waitlist is full")
	}

	// Upgrading lifts it.
	a.db.W.Exec(`UPDATE admins SET plan = 'launch' WHERE id = ?`, id)
	if _, err := a.SubmitSignup(ctx, p, SignupInput{Email: "over@example.com", DwellMs: -1}); err != nil {
		t.Fatalf("launch has no signup cap: %v", err)
	}
}

func TestCloudEmailCap(t *testing.T) {
	a := newCloudApp(t)
	ctx := context.Background()
	id, _ := a.SignUp(ctx, "owner@example.com", "correct horse battery")
	p, _, _ := a.CreateProject(ctx, id, "Orbit", "orbit")
	a.db.W.Exec(`UPDATE projects SET doi_enabled = 0 WHERE id = ?`, p.ID)
	p, _ = a.ProjectByID(ctx, p.ID)
	for i := range 3 {
		signup(t, a, p, fmt.Sprintf("p%d@example.com", i), "", "1.1.1."+strconv.Itoa(i))
	}
	a.runJobs(ctx)
	if u := a.UsageOf(ctx, id); u.Emails != 3 { // the status emails; owner notes are free
		t.Fatalf("emails counted: %d", u.Emails)
	}
	a.db.W.Exec(`UPDATE email_usage SET sent = 999 WHERE admin_id = ?`, id)
	if _, err := a.Broadcast(ctx, p, "Hi", "Launching", "confirmed"); err == nil {
		t.Fatal("broadcast over the cap should be refused")
	}
	a.db.W.Exec(`UPDATE email_usage SET sent = 900 WHERE admin_id = ?`, id)
	if n, err := a.Broadcast(ctx, p, "Hi", "Launching", "confirmed"); err != nil || n != 3 {
		t.Fatalf("broadcast within the cap: %d %v", n, err)
	}
}

func polarEvent(t *testing.T, h http.Handler, secret, msgID, typ string, sub map[string]any) int {
	body, _ := json.Marshal(map[string]any{"type": typ, "data": sub})
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest("POST", "/v1/hooks/polar", strings.NewReader(string(body)))
	req.Header.Set("webhook-id", msgID)
	req.Header.Set("webhook-timestamp", ts)
	req.Header.Set("webhook-signature", SignWebhook(secret, msgID, ts, body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestPolarWebhook(t *testing.T) {
	a := newCloudApp(t)
	srv, _ := NewServer(a)
	h := srv.Handler()
	ctx := context.Background()
	id, _ := a.SignUp(ctx, "owner@example.com", "correct horse battery")
	secret := a.cfg.Polar.WebhookSecret
	sub := map[string]any{
		"id": "sub_1", "status": "active", "product_id": "prod-launch-m", "customer_id": "cus_1",
		"current_period_end": "2026-11-28T10:00:00Z", "cancel_at_period_end": false,
		"customer": map[string]any{"external_id": externalID(id)},
	}
	if code := polarEvent(t, h, "whsec_d3Jvbmc=", "m0", "subscription.created", sub); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	if code := polarEvent(t, h, secret, "m1", "subscription.active", sub); code != 202 {
		t.Fatalf("active: %d", code)
	}
	ac, _ := a.Account(ctx, id)
	if ac.Plan != "launch" || ac.Interval != "month" || ac.PolarCustomerID != "cus_1" || ac.PeriodEnd == nil {
		t.Fatalf("after active: %+v", ac)
	}
	// Cancelling keeps the plan until the period ends.
	sub["cancel_at_period_end"] = true
	polarEvent(t, h, secret, "m2", "subscription.canceled", sub)
	if ac, _ = a.Account(ctx, id); ac.Plan != "launch" || !ac.CancelAtPeriodEnd {
		t.Fatalf("after cancel: %+v", ac)
	}
	// Revoked drops to free.
	sub["status"] = "canceled"
	polarEvent(t, h, secret, "m3", "subscription.revoked", sub)
	if ac, _ = a.Account(ctx, id); ac.Plan != "free" {
		t.Fatalf("after revoke: %+v", ac)
	}
	// A replayed event is ignored.
	sub["status"] = "active"
	polarEvent(t, h, secret, "m1", "subscription.active", sub)
	if ac, _ = a.Account(ctx, id); ac.Plan != "free" {
		t.Fatal("a replayed webhook id must not apply twice")
	}
	// Legacy secrets were used as raw bytes.
	body := []byte(`{"type":"subscription.active","data":{"id":"sub_2","status":"active","product_id":"prod-pro-y","customer_id":"cus_1","customer":{"external_id":"` + externalID(id) + `"}}}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m := legacySign(secret, "m4", ts, body)
	if !verifyPolar(secret, "m4", ts, m, body) {
		t.Fatal("legacy signature should verify")
	}
}

// legacySign signs with the secret's raw bytes as the key, as Polar did before September 2026.
func legacySign(secret, id, ts string, body []byte) string {
	return SignWebhook("whsec_"+base64.StdEncoding.EncodeToString([]byte(secret)), id, ts, body)
}
