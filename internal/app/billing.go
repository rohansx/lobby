package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

// Billing runs through Polar (polar.sh), which is the merchant of record:
// it takes the payment, handles sales tax and hosts the customer portal.
// Lobby only creates checkouts and portal sessions, and keeps each account's
// plan in step with Polar's subscription webhooks.

var polarClient = &http.Client{Timeout: 20 * time.Second}

func (p PolarConfig) base() string {
	if p.APIURL != "" {
		return p.APIURL
	}
	if p.Sandbox {
		return "https://sandbox-api.polar.sh"
	}
	return "https://api.polar.sh"
}

// polar calls the Polar API: a POST with body, or a GET when body is nil.
func (a *App) polar(ctx context.Context, path string, body, out any) error {
	method, rd := http.MethodGet, io.Reader(nil)
	if body != nil {
		b, _ := json.Marshal(body)
		method, rd = http.MethodPost, bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.cfg.Polar.base()+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.Polar.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := polarClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("polar %s: HTTP %d: %s", path, resp.StatusCode, clip(strings.TrimSpace(string(raw)), 500))
	}
	return json.Unmarshal(raw, out)
}

// externalID ties a Polar customer to an account.
func externalID(id int64) string { return "lobby_" + strconv.FormatInt(id, 10) }

func accountFromExternal(ext string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(ext, "lobby_"), 10, 64)
	return id, err == nil && strings.HasPrefix(ext, "lobby_")
}

var errBillingOff = errors.New("Billing isn't set up on this instance yet.")

// Checkout returns a Polar checkout URL for a plan ("launch" or "pro") and
// interval ("month" or "year").
func (a *App) Checkout(ctx context.Context, ac *Account, plan, interval string) (string, error) {
	if !a.cfg.Polar.Enabled() {
		return "", errBillingOff
	}
	product := a.cfg.Polar.Products[plan+"_"+interval]
	if product == "" {
		return "", errors.New("unknown plan")
	}
	if ac.PolarSubscription != "" && paidStatus(ac.SubStatus) {
		return "", errors.New("You already have a subscription. Change plans from Manage billing.")
	}
	var out struct {
		URL string `json:"url"`
	}
	err := a.polar(ctx, "/v1/checkouts/", map[string]any{
		"products":             []string{product},
		"customer_email":       ac.Email,
		"external_customer_id": externalID(ac.ID),
		"success_url":          a.cfg.BaseURL + "/billing?checkout={CHECKOUT_ID}",
		"return_url":           a.cfg.BaseURL + "/billing",
		"metadata":             map[string]string{"account_id": strconv.FormatInt(ac.ID, 10)},
	}, &out)
	return out.URL, err
}

// Portal returns a link to Polar's customer portal, where the customer
// changes plan, updates their card, downloads invoices or cancels.
func (a *App) Portal(ctx context.Context, ac *Account) (string, error) {
	if !a.cfg.Polar.Enabled() {
		return "", errBillingOff
	}
	body := map[string]any{"return_url": a.cfg.BaseURL + "/billing"}
	if ac.PolarCustomerID != "" {
		body["customer_id"] = ac.PolarCustomerID
	} else {
		body["external_customer_id"] = externalID(ac.ID)
	}
	var out struct {
		URL string `json:"customer_portal_url"`
	}
	err := a.polar(ctx, "/v1/customer-sessions/", body, &out)
	return out.URL, err
}

// paidStatus is true for the subscription states that keep a paid plan.
// past_due keeps it while Polar retries the card.
func paidStatus(s string) bool { return s == "active" || s == "trialing" || s == "past_due" }

// planForProduct maps a Polar product back to a plan and interval.
func (a *App) planForProduct(product string) (string, string, bool) {
	for k, id := range a.cfg.Polar.Products {
		if id == product {
			plan, interval, _ := strings.Cut(k, "_")
			return plan, interval, true
		}
	}
	return "", "", false
}

type polarSubscription struct {
	ID                string         `json:"id"`
	Status            string         `json:"status"`
	ProductID         string         `json:"product_id"`
	CustomerID        string         `json:"customer_id"`
	CurrentPeriodEnd  *time.Time     `json:"current_period_end"`
	CancelAtPeriodEnd bool           `json:"cancel_at_period_end"`
	Metadata          map[string]any `json:"metadata"`
	Customer          struct {
		ExternalID *string `json:"external_id"`
	} `json:"customer"`
}

// ApplySubscription brings an account's plan in line with a subscription.
// An older subscription ending never overrides a newer paid one.
func (a *App) ApplySubscription(ctx context.Context, sub *polarSubscription) error {
	var id int64
	if sub.Customer.ExternalID != nil {
		id, _ = accountFromExternal(*sub.Customer.ExternalID)
	}
	if id == 0 {
		if v, ok := sub.Metadata["account_id"].(string); ok {
			id, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	if id == 0 {
		a.db.W.QueryRowContext(ctx, `SELECT id FROM admins WHERE polar_customer_id = ?`, sub.CustomerID).Scan(&id)
	}
	ac, err := a.Account(ctx, id)
	if err != nil {
		a.log.Warn("polar: subscription for unknown account", "subscription", sub.ID)
		return nil
	}
	if ac.PolarSubscription != "" && ac.PolarSubscription != sub.ID && paidStatus(ac.SubStatus) && !paidStatus(sub.Status) {
		return nil // an old subscription ending doesn't touch the current one
	}
	plan, interval := "free", ""
	if paidStatus(sub.Status) {
		p, iv, ok := a.planForProduct(sub.ProductID)
		if !ok {
			return fmt.Errorf("%w: polar product %s isn't one of the configured plans", ErrPermanent, sub.ProductID)
		}
		plan, interval = p, iv
	}
	var periodEnd *int64
	if sub.CurrentPeriodEnd != nil {
		t := sub.CurrentPeriodEnd.Unix()
		periodEnd = &t
	}
	_, err = a.db.W.ExecContext(ctx, `UPDATE admins SET plan = ?, plan_interval = ?, sub_status = ?, period_end = ?,
		cancel_at_period_end = ?, polar_customer_id = ?, polar_subscription_id = ? WHERE id = ?`,
		plan, interval, sub.Status, periodEnd, sub.CancelAtPeriodEnd, sub.CustomerID, sub.ID, ac.ID)
	if err == nil {
		a.log.Info("billing: plan updated", "account", ac.ID, "plan", plan, "status", sub.Status)
	}
	return err
}

// verifyPolar checks Polar's Standard Webhooks signature. Secrets made before
// September 2026 were used as raw UTF-8 bytes rather than base64, so both
// readings of the secret are accepted.
func verifyPolar(secret, id, ts, sigHeader string, body []byte) bool {
	if VerifyWebhook(secret, id, ts, sigHeader, body) {
		return true
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || abs(time.Now().Unix()-t) > 300 {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(id + "." + ts + "."))
	m.Write(body)
	want := "v1," + base64.StdEncoding.EncodeToString(m.Sum(nil))
	for _, s := range strings.Fields(sigHeader) {
		if hmac.Equal([]byte(s), []byte(want)) {
			return true
		}
	}
	return false
}

func (s *Server) polarHook(w http.ResponseWriter, r *http.Request) {
	secret := s.app.cfg.Polar.WebhookSecret
	if secret == "" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	id, ts, sig := r.Header.Get("webhook-id"), r.Header.Get("webhook-timestamp"), r.Header.Get("webhook-signature")
	if !verifyPolar(secret, id, ts, sig, body) {
		http.Error(w, "bad signature", 401)
		return
	}
	ctx := r.Context()
	var seen int
	s.app.db.W.QueryRowContext(ctx, `SELECT COUNT(*) FROM billing_events WHERE id = ?`, id).Scan(&seen)
	if seen > 0 {
		w.WriteHeader(202)
		return
	}
	var ev struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if strings.HasPrefix(ev.Type, "subscription.") {
		var sub polarSubscription
		if err := json.Unmarshal(ev.Data, &sub); err != nil {
			http.Error(w, "bad subscription", 400)
			return
		}
		// Webhooks can arrive out of order, so act on the subscription as it
		// is now rather than as the event describes it.
		if s.app.cfg.Polar.Token != "" {
			var fresh polarSubscription
			if err := s.app.polar(ctx, "/v1/subscriptions/"+url.PathEscape(sub.ID), nil, &fresh); err != nil {
				s.app.log.Error("polar webhook: fetch subscription", "err", err)
				http.Error(w, "error", 502)
				return
			}
			sub = fresh
		}
		if err := s.app.ApplySubscription(ctx, &sub); err != nil {
			s.app.log.Error("polar webhook", "type", ev.Type, "err", err)
			http.Error(w, "error", 500)
			return
		}
	}
	s.app.db.W.ExecContext(ctx, `INSERT OR IGNORE INTO billing_events (id, created_at) VALUES (?, ?)`, id, now())
	w.WriteHeader(202)
}
