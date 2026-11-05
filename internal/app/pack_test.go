package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakePolar answers checkouts and order/subscription lookups from maps the
// test fills in, and records the last checkout request body.
type fakePolar struct {
	mu       sync.Mutex
	checkout map[string]any
	orders   map[string]map[string]any
	subs     map[string]map[string]any
}

func newFakePolar(t *testing.T, a *App) *fakePolar {
	f := &fakePolar{orders: map[string]map[string]any{}, subs: map[string]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer polar_test" {
			http.Error(w, "unauthorized", 401)
			return
		}
		var body map[string]any
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/checkouts/":
			json.NewDecoder(r.Body).Decode(&f.checkout)
			body = map[string]any{"url": "https://polar.test/checkout/ch_pack"}
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/orders/"):
			body = f.orders[strings.TrimPrefix(r.URL.Path, "/v1/orders/")]
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/"):
			body = f.subs[strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")]
		}
		if body == nil {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	a.cfg.Polar.Token, a.cfg.Polar.APIURL, a.cfg.Polar.LaunchPack = "polar_test", srv.URL, "prod-pack"
	return f
}

func (f *fakePolar) setOrder(o map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orders[o["id"].(string)] = o
}

func TestPackCheckoutAndOrderPaid(t *testing.T) {
	a := newCloudApp(t)
	f := newFakePolar(t, a)
	srv, _ := NewServer(a)
	h := srv.Handler()
	c := &client{t: t, h: h}
	if code, out := c.do("POST", "/api/admin/signup", `{"email":"buyer@example.com","password":"correct horse battery"}`); code != 200 {
		t.Fatalf("signup: %d %v", code, out)
	}
	var id int64
	a.db.R.QueryRow(`SELECT id FROM admins WHERE email = 'buyer@example.com'`).Scan(&id)

	code, out := c.do("POST", "/api/admin/billing/pack-checkout", ``)
	if code != 200 || out["url"] != "https://polar.test/checkout/ch_pack" {
		t.Fatalf("pack checkout: %d %v", code, out)
	}
	f.mu.Lock()
	ck := f.checkout
	f.mu.Unlock()
	if p, _ := ck["products"].([]any); len(p) != 1 || p[0] != "prod-pack" {
		t.Fatalf("checkout products: %v", ck)
	}
	if ck["external_customer_id"] != externalID(id) || ck["customer_email"] != "buyer@example.com" ||
		!strings.HasPrefix(ck["success_url"].(string), "http://lobby.test/campaigns") {
		t.Fatalf("checkout request: %v", ck)
	}

	secret := a.cfg.Polar.WebhookSecret
	ext := externalID(id)
	f.setOrder(map[string]any{"id": "ord_1", "status": "paid", "paid": true, "product_id": "prod-pack",
		"customer_id": "cus_9", "net_amount": 9900, "currency": "usd", "customer": map[string]any{"external_id": ext}})
	// The body is a stub: the handler must act on the order Polar returns.
	stub := map[string]any{"id": "ord_1"}
	if code := polarEvent(t, h, secret, "o1", "order.paid", stub); code != 202 {
		t.Fatalf("order.paid: %d", code)
	}
	// Same order under a new webhook id (Polar retry) and the same webhook id again.
	polarEvent(t, h, secret, "o2", "order.paid", stub)
	polarEvent(t, h, secret, "o1", "order.paid", stub)
	var n int
	var price int64
	a.db.R.QueryRow(`SELECT COUNT(*), COALESCE(MAX(price_cents), 0) FROM campaigns WHERE owner_id = ?`, id).Scan(&n, &price)
	if n != 1 || price != 9900 {
		t.Fatalf("want exactly one campaign at 9900, got %d at %d", n, price)
	}
	ac, _ := a.Account(t.Context(), id)
	if ac.Plan != "pack" || ac.PolarCustomerID != "cus_9" {
		t.Fatalf("after order.paid: %+v", ac)
	}

	code, out = c.do("GET", "/api/admin/billing", "")
	if packs, _ := out["packs"].([]any); code != 200 || len(packs) != 1 || out["subscriber"] != false {
		t.Fatalf("billing: %d %v", code, out)
	}

	// An order for another product (a legacy subscription renewal) does nothing.
	f.setOrder(map[string]any{"id": "ord_2", "status": "paid", "paid": true, "product_id": "prod-launch-m",
		"customer_id": "cus_9", "net_amount": 500, "customer": map[string]any{"external_id": ext}})
	if code := polarEvent(t, h, secret, "o3", "order.paid", map[string]any{"id": "ord_2"}); code != 202 {
		t.Fatalf("other product: %d", code)
	}
	a.db.R.QueryRow(`SELECT COUNT(*) FROM campaigns`).Scan(&n)
	if n != 1 {
		t.Fatalf("a non-pack order must not open a campaign, have %d", n)
	}

	// A refund alerts the operators and changes nothing.
	a.cfg.Operators = []string{"ops@example.com"}
	f.setOrder(map[string]any{"id": "ord_1", "status": "refunded", "paid": true, "product_id": "prod-pack",
		"customer_id": "cus_9", "net_amount": 9900, "customer": map[string]any{"external_id": ext}})
	if code := polarEvent(t, h, secret, "o4", "order.refunded", stub); code != 202 {
		t.Fatalf("order.refunded: %d", code)
	}
	a.db.R.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE status != 'cancelled'`).Scan(&n)
	if ac, _ := a.Account(t.Context(), id); n != 1 || ac.Plan != "pack" {
		t.Fatalf("a refund must not touch the campaign or plan: %d %s", n, ac.Plan)
	}
	if _, err := a.runJobs(t.Context()); err != nil {
		t.Fatal(err)
	}
	msgs := a.mailer.(*captureMailer).snapshot()
	if !mailTo(msgs, "ops@example.com", "Refund: buyer@example.com") || !mailTo(msgs, "buyer@example.com", "") {
		t.Fatalf("want the founder welcome and the operator refund emails: %+v", msgs)
	}

	// A late order.paid for an order already refunded (Polar keeps paid=true) opens nothing.
	f.setOrder(map[string]any{"id": "ord_3", "status": "refunded", "paid": true, "product_id": "prod-pack",
		"customer_id": "cus_9", "net_amount": 9900, "customer": map[string]any{"external_id": ext}})
	if code := polarEvent(t, h, secret, "o5", "order.paid", map[string]any{"id": "ord_3"}); code != 202 {
		t.Fatalf("refunded order.paid: %d", code)
	}
	a.db.R.QueryRow(`SELECT COUNT(*) FROM campaigns`).Scan(&n)
	if n != 1 {
		t.Fatalf("a refunded order opened a campaign: %d", n)
	}
}

func TestPackCheckoutNeedsBilling(t *testing.T) {
	a := newCloudApp(t)
	srv, _ := NewServer(a)
	c := &client{t: t, h: srv.Handler()}
	c.do("POST", "/api/admin/signup", `{"email":"x@example.com","password":"correct horse battery"}`)
	if code, out := c.do("POST", "/api/admin/billing/pack-checkout", ``); code != 400 || out["error"] != errBillingOff.Error() {
		t.Fatalf("pack checkout without Polar: %d %v", code, out)
	}
}

// A grandfathered subscriber's webhooks keep working after the pivot, and
// buying a pack doesn't take their subscription plan away.
func TestGrandfatheredSubscription(t *testing.T) {
	a := newCloudApp(t)
	f := newFakePolar(t, a)
	srv, _ := NewServer(a)
	h := srv.Handler()
	ctx := context.Background()
	id, _ := a.SignUp(ctx, "old@example.com", "correct horse battery")
	secret := a.cfg.Polar.WebhookSecret
	sub := map[string]any{"id": "sub_7", "status": "active", "product_id": "prod-pro-y", "customer_id": "cus_7",
		"customer": map[string]any{"external_id": externalID(id)}}
	f.mu.Lock()
	f.subs["sub_7"] = sub
	f.mu.Unlock()
	if code := polarEvent(t, h, secret, "s1", "subscription.updated", sub); code != 202 {
		t.Fatalf("subscription.updated: %d", code)
	}
	if ac, _ := a.Account(ctx, id); ac.Plan != "pro" || ac.Interval != "year" {
		t.Fatalf("grandfathered plan: %+v", ac)
	}
	f.setOrder(map[string]any{"id": "ord_7", "status": "paid", "paid": true, "product_id": "prod-pack",
		"customer_id": "cus_7", "net_amount": 9900, "customer": map[string]any{"external_id": externalID(id)}})
	polarEvent(t, h, secret, "s2", "order.paid", map[string]any{"id": "ord_7"})
	if ac, _ := a.Account(ctx, id); ac.Plan != "pro" {
		t.Fatalf("a pack must not replace an active subscription: %s", ac.Plan)
	}
	c := &client{t: t, h: h}
	c.do("POST", "/api/admin/login", `{"email":"old@example.com","password":"correct horse battery"}`)
	if code, out := c.do("GET", "/api/admin/billing", ""); code != 200 || out["subscriber"] != true {
		t.Fatalf("billing for a subscriber: %d %v", code, out)
	}
	f.mu.Lock()
	sub["status"] = "canceled"
	f.mu.Unlock()
	polarEvent(t, h, secret, "s3", "subscription.revoked", sub)
	if ac, _ := a.Account(ctx, id); ac.Plan != "pack" {
		t.Fatalf("an ended subscription falls back to pack: %s", ac.Plan)
	}
}
