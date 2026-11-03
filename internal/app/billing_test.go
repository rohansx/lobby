package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
)

// TestCloudMoneyPath runs signup -> verify -> checkout -> paid webhook ->
// higher limits -> cancel/revoke -> free, all through the HTTP handler and a
// fake Polar API.
func TestCloudMoneyPath(t *testing.T) {
	a := newCloudApp(t)
	var mu sync.Mutex
	var checkout map[string]any
	sub := map[string]any{}
	fetches := 0
	polar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer polar_test" {
			http.Error(w, "unauthorized", 401)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/checkouts/":
			json.NewDecoder(r.Body).Decode(&checkout)
			json.NewEncoder(w).Encode(map[string]any{"url": "https://polar.test/checkout/ch_1"})
		case r.Method == "GET" && r.URL.Path == "/v1/subscriptions/sub_1":
			fetches++
			json.NewEncoder(w).Encode(sub)
		default:
			http.NotFound(w, r)
		}
	}))
	defer polar.Close()
	a.cfg.Polar.Token, a.cfg.Polar.APIURL = "polar_test", polar.URL
	// setSub changes what Polar reports; the webhook handler re-fetches it.
	setSub := func(kv ...any) {
		mu.Lock()
		defer mu.Unlock()
		for i := 0; i < len(kv); i += 2 {
			sub[kv[i].(string)] = kv[i+1]
		}
	}

	srv, _ := NewServer(a)
	h := srv.Handler()
	c := &client{t: t, h: h}
	if code, out := c.do("POST", "/api/admin/signup", `{"email":"payer@example.com","password":"correct horse battery"}`); code != 200 {
		t.Fatalf("signup: %d %v", code, out)
	}
	mails := a.mailer.(*captureMailer).sent
	if len(mails) != 1 {
		t.Fatalf("want one verification email, got %d", len(mails))
	}
	tok := regexp.MustCompile(`/verify/(\S+)`).FindStringSubmatch(mails[0].Text)
	if tok == nil {
		t.Fatalf("no verify link in %q", mails[0].Text)
	}
	c.do("GET", "/verify/"+tok[1], "")
	var id int64
	a.db.R.QueryRow(`SELECT id FROM admins WHERE email = 'payer@example.com'`).Scan(&id)
	if ac, _ := a.Account(t.Context(), id); !ac.Verified {
		t.Fatal("account should be verified after the link")
	}

	if code, out := c.do("POST", "/api/admin/projects", `{"name":"One","slug":"one"}`); code != 201 {
		t.Fatalf("first waitlist: %d %v", code, out)
	}
	if code, _ := c.do("POST", "/api/admin/projects", `{"name":"Two","slug":"two"}`); code != http.StatusPaymentRequired {
		t.Fatalf("free plan must refuse a second waitlist: %d", code)
	}

	code, out := c.do("POST", "/api/admin/billing/checkout", `{"plan":"launch","interval":"month"}`)
	if code != 200 || out["url"] != "https://polar.test/checkout/ch_1" {
		t.Fatalf("checkout: %d %v", code, out)
	}
	mu.Lock()
	if p, _ := checkout["products"].([]any); len(p) != 1 || p[0] != "prod-launch-m" || checkout["external_customer_id"] != externalID(id) {
		t.Fatalf("checkout request: %v", checkout)
	}
	mu.Unlock()

	secret := a.cfg.Polar.WebhookSecret
	setSub("id", "sub_1", "status", "active", "product_id", "prod-launch-m", "customer_id", "cus_1",
		"current_period_end", "2026-12-03T10:00:00Z", "cancel_at_period_end", false,
		"customer", map[string]any{"external_id": externalID(id)})
	if code := polarEvent(t, h, "whsec_d3Jvbmc=", "e0", "subscription.active", sub); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	if ac, _ := a.Account(t.Context(), id); ac.Plan != "free" {
		t.Fatal("a badly signed webhook must not change the plan")
	}
	if code := polarEvent(t, h, secret, "e1", "subscription.active", sub); code != 202 {
		t.Fatalf("active: %d", code)
	}
	if ac, _ := a.Account(t.Context(), id); ac.Plan != "launch" || ac.PolarSubscription != "sub_1" {
		t.Fatalf("after active: %+v", ac)
	}
	if code, out := c.do("POST", "/api/admin/projects", `{"name":"Two","slug":"two"}`); code != 201 {
		t.Fatalf("launch allows a second waitlist: %d %v", code, out)
	}
	if code, _ := c.do("POST", "/api/admin/billing/checkout", `{"plan":"pro","interval":"year"}`); code != 400 {
		t.Fatalf("a second checkout while subscribed must fail: %d", code)
	}

	// Cancelling keeps the plan until the period ends.
	setSub("cancel_at_period_end", true)
	polarEvent(t, h, secret, "e2", "subscription.canceled", sub)
	if ac, _ := a.Account(t.Context(), id); ac.Plan != "launch" || !ac.CancelAtPeriodEnd {
		t.Fatalf("after cancel: %+v", ac)
	}
	// Revoked drops to free, and the third waitlist is refused again.
	setSub("status", "canceled")
	polarEvent(t, h, secret, "e3", "subscription.revoked", sub)
	if ac, _ := a.Account(t.Context(), id); ac.Plan != "free" {
		t.Fatalf("after revoke: %+v", ac)
	}
	if code, _ := c.do("POST", "/api/admin/projects", `{"name":"Three","slug":"three"}`); code != http.StatusPaymentRequired {
		t.Fatalf("free plan again must refuse more waitlists: %d", code)
	}

	// A replayed webhook id is acknowledged but not applied, nor re-fetched.
	setSub("status", "active")
	mu.Lock()
	before := fetches
	mu.Unlock()
	if before != 3 {
		t.Fatalf("each signed webhook should re-fetch the subscription once, got %d fetches", before)
	}
	if code := polarEvent(t, h, secret, "e1", "subscription.active", sub); code != 202 {
		t.Fatalf("replay: %d", code)
	}
	mu.Lock()
	if fetches != before {
		t.Fatal("a replayed webhook must not reach Polar")
	}
	mu.Unlock()
	if ac, _ := a.Account(t.Context(), id); ac.Plan != "free" {
		t.Fatal("a replayed webhook id must not apply twice")
	}
	var events int
	a.db.R.QueryRow(`SELECT COUNT(*) FROM billing_events`).Scan(&events)
	if events != 3 {
		t.Fatalf("want 3 recorded events, got %d", events)
	}
}
