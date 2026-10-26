package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Events a webhook can subscribe to.
var Events = []string{"signup.created", "signup.confirmed", "signup.referred", "signup.invited", "signup.joined", "signup.removed"}

type Webhook struct {
	ID             int64  `json:"id"`
	ProjectID      int64  `json:"-"`
	URL            string `json:"url"`
	Secret         string `json:"secret,omitempty"`
	Kind           string `json:"kind"` // standard or slack
	Events         string `json:"events"`
	Active         bool   `json:"active"`
	DisabledReason string `json:"disabled_reason"`
	CreatedAt      int64  `json:"created_at"`
}

type Delivery struct {
	ID           int64  `json:"id"`
	WebhookID    int64  `json:"webhook_id"`
	MsgID        string `json:"msg_id"`
	Event        string `json:"event"`
	Payload      string `json:"payload"`
	Status       string `json:"status"`
	Attempts     int    `json:"attempts"`
	NextAt       int64  `json:"next_at"`
	ResponseCode int    `json:"response_code"`
	LastError    string `json:"last_error"`
	CreatedAt    int64  `json:"created_at"`
}

func (w *Webhook) Wants(event string) bool {
	if w.Events == "*" || w.Events == "" {
		return true
	}
	for _, e := range strings.Split(w.Events, ",") {
		if strings.TrimSpace(e) == event {
			return true
		}
	}
	return false
}

func (a *App) CreateWebhook(ctx context.Context, p *Project, rawURL, kind string, events []string) (*Webhook, error) {
	if !isHTTPURL(rawURL) {
		return nil, errors.New("webhook URL must start with http:// or https://")
	}
	if kind != "slack" {
		kind = "standard"
	}
	ev := "*"
	if len(events) > 0 {
		for _, e := range events {
			if !contains(Events, e) {
				return nil, fmt.Errorf("unknown event %q", e)
			}
		}
		ev = strings.Join(events, ",")
	}
	key := make([]byte, 24)
	rand.Read(key)
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	res, err := a.db.W.ExecContext(ctx, `INSERT INTO webhooks (project_id, url, secret, kind, events, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, rawURL, secret, kind, ev, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return a.Webhook(ctx, p, id)
}

func (a *App) Webhook(ctx context.Context, p *Project, id int64) (*Webhook, error) {
	w := &Webhook{}
	err := a.db.W.QueryRowContext(ctx, `SELECT id, project_id, url, secret, kind, events, active, disabled_reason, created_at
		FROM webhooks WHERE project_id = ? AND id = ?`, p.ID, id).
		Scan(&w.ID, &w.ProjectID, &w.URL, &w.Secret, &w.Kind, &w.Events, &w.Active, &w.DisabledReason, &w.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return w, err
}

func (a *App) Webhooks(ctx context.Context, p *Project) ([]*Webhook, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT id, project_id, url, secret, kind, events, active, disabled_reason, created_at
		FROM webhooks WHERE project_id = ? ORDER BY id`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Webhook
	for rows.Next() {
		w := &Webhook{}
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.URL, &w.Secret, &w.Kind, &w.Events, &w.Active, &w.DisabledReason, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (a *App) SetWebhookActive(ctx context.Context, p *Project, id int64, active bool) error {
	_, err := a.db.W.ExecContext(ctx, `UPDATE webhooks SET active = ?, disabled_reason = '' WHERE project_id = ? AND id = ?`, active, p.ID, id)
	return err
}

func (a *App) DeleteWebhook(ctx context.Context, p *Project, id int64) error {
	_, err := a.db.W.ExecContext(ctx, `DELETE FROM webhooks WHERE project_id = ? AND id = ?`, p.ID, id)
	return err
}

func (a *App) Deliveries(ctx context.Context, p *Project, limit int) ([]*Delivery, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT d.id, d.webhook_id, d.msg_id, d.event, d.payload, d.status, d.attempts,
		d.next_at, d.response_code, d.last_error, d.created_at FROM deliveries d JOIN webhooks w ON w.id = d.webhook_id
		WHERE w.project_id = ? ORDER BY d.id DESC LIMIT ?`, p.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Delivery
	for rows.Next() {
		d := &Delivery{}
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.MsgID, &d.Event, &d.Payload, &d.Status, &d.Attempts, &d.NextAt,
			&d.ResponseCode, &d.LastError, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Redeliver queues a delivery to go out again immediately.
func (a *App) Redeliver(ctx context.Context, p *Project, deliveryID int64) error {
	res, err := a.db.W.ExecContext(ctx, `UPDATE deliveries SET status = 'pending', next_at = ?, updated_at = ?
		WHERE id = ? AND webhook_id IN (SELECT id FROM webhooks WHERE project_id = ?)`, now(), now(), deliveryID, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	a.kick()
	return nil
}

// TestWebhook sends a sample event to one webhook through the normal queue.
func (a *App) TestWebhook(ctx context.Context, p *Project, id int64) error {
	w, err := a.Webhook(ctx, p, id)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"type":      "webhook.test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      map[string]any{"project": p.Slug, "message": "Test event from Lobby"},
	})
	_, err = a.db.W.ExecContext(ctx, `INSERT INTO deliveries (webhook_id, msg_id, event, payload, next_at, created_at, updated_at)
		VALUES (?, ?, 'webhook.test', ?, ?, ?, ?)`, w.ID, "msg_"+randHex(12), string(body), now(), now(), now())
	a.kick()
	return err
}

// emit records one delivery per subscribed webhook inside the caller's
// transaction, so an event exists if and only if the change it describes does.
func (a *App) emit(ctx context.Context, tx *sql.Tx, projectID int64, event string, data any) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, events FROM webhooks WHERE project_id = ? AND active = 1`, projectID)
	if err != nil {
		return err
	}
	var targets []int64
	for rows.Next() {
		w := &Webhook{}
		if err := rows.Scan(&w.ID, &w.Events); err != nil {
			rows.Close()
			return err
		}
		if w.Wants(event) {
			targets = append(targets, w.ID)
		}
	}
	rows.Close()
	if len(targets) == 0 {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"type":      event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	})
	if err != nil {
		return err
	}
	msgID := "msg_" + randHex(12)
	for _, id := range targets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO deliveries (webhook_id, msg_id, event, payload, next_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, id, msgID, event, string(body), now(), now(), now()); err != nil {
			return err
		}
	}
	return nil
}

// Retry schedule after each failed attempt, per the Standard Webhooks guidance.
var webhookBackoff = []time.Duration{5 * time.Second, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 5 * time.Hour, 10 * time.Hour, 10 * time.Hour}

var webhookClient = &http.Client{Timeout: 15 * time.Second}

func (a *App) deliverDue(ctx context.Context) (int, error) {
	rows, err := a.db.W.QueryContext(ctx, `SELECT d.id, d.msg_id, d.event, d.payload, d.attempts, w.id, w.url, w.secret, w.kind, w.active
		FROM deliveries d JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.status = 'pending' AND d.next_at <= ? ORDER BY d.next_at LIMIT 20`, now())
	if err != nil {
		return 0, err
	}
	type due struct {
		d Delivery
		w Webhook
	}
	var batch []due
	for rows.Next() {
		var x due
		if err := rows.Scan(&x.d.ID, &x.d.MsgID, &x.d.Event, &x.d.Payload, &x.d.Attempts, &x.w.ID, &x.w.URL, &x.w.Secret, &x.w.Kind, &x.w.Active); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, x)
	}
	rows.Close()
	for _, x := range batch {
		if !x.w.Active {
			a.db.W.ExecContext(ctx, `UPDATE deliveries SET status = 'skipped', updated_at = ? WHERE id = ?`, now(), x.d.ID)
			continue
		}
		code, err := a.send(ctx, &x.w, &x.d)
		attempts := x.d.Attempts + 1
		switch {
		case err == nil && code >= 200 && code < 300:
			a.db.W.ExecContext(ctx, `UPDATE deliveries SET status = 'success', attempts = ?, response_code = ?, last_error = '', updated_at = ? WHERE id = ?`,
				attempts, code, now(), x.d.ID)
		case code == http.StatusGone:
			a.db.W.ExecContext(ctx, `UPDATE deliveries SET status = 'failed', attempts = ?, response_code = ?, last_error = 'endpoint returned 410, webhook disabled', updated_at = ? WHERE id = ?`,
				attempts, code, now(), x.d.ID)
			a.db.W.ExecContext(ctx, `UPDATE webhooks SET active = 0, disabled_reason = 'endpoint returned 410 Gone' WHERE id = ?`, x.w.ID)
		default:
			msg := fmt.Sprintf("HTTP %d", code)
			if err != nil {
				msg = err.Error()
			}
			status, next := "pending", now()
			if attempts > len(webhookBackoff) {
				status = "failed"
			} else {
				next += int64(webhookBackoff[attempts-1].Seconds())
			}
			a.db.W.ExecContext(ctx, `UPDATE deliveries SET status = ?, attempts = ?, response_code = ?, last_error = ?, next_at = ?, updated_at = ? WHERE id = ?`,
				status, attempts, code, clip(msg, 500), next, now(), x.d.ID)
		}
	}
	return len(batch), nil
}

func (a *App) send(ctx context.Context, w *Webhook, d *Delivery) (int, error) {
	body := []byte(d.Payload)
	if w.Kind == "slack" {
		body, _ = json.Marshal(map[string]string{"text": slackText(d)})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Lobby-Webhooks/1")
	if w.Kind == "standard" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("webhook-id", d.MsgID)
		req.Header.Set("webhook-timestamp", ts)
		req.Header.Set("webhook-signature", SignWebhook(w.Secret, d.MsgID, ts, body))
	}
	resp, err := webhookClient.Do(req)
	if err != nil {
		return 0, err
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	return resp.StatusCode, nil
}

func webhookKey(secret string) []byte {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return []byte(secret)
	}
	return key
}

// SignWebhook implements the Standard Webhooks v1 signature:
// base64(HMAC-SHA256(key, "{id}.{timestamp}.{body}")).
func SignWebhook(secret, msgID, ts string, body []byte) string {
	m := hmac.New(sha256.New, webhookKey(secret))
	m.Write([]byte(msgID + "." + ts + "."))
	m.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// VerifyWebhook checks a Standard Webhooks (or Svix) signature header, which
// may carry several space-separated signatures, within a 5 minute tolerance.
func VerifyWebhook(secret, msgID, ts, sigHeader string, body []byte) bool {
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || abs(time.Now().Unix()-t) > 300 {
		return false
	}
	want := SignWebhook(secret, msgID, ts, body)
	for _, s := range strings.Fields(sigHeader) {
		if hmac.Equal([]byte(s), []byte(want)) {
			return true
		}
	}
	return false
}

func slackText(d *Delivery) string {
	var ev struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal([]byte(d.Payload), &ev)
	var s struct {
		Email  string `json:"email"`
		Signup *struct {
			Email string `json:"email"`
		} `json:"signup"`
		Referrer *struct {
			Email string `json:"email"`
		} `json:"referrer"`
		Referred *struct {
			Email string `json:"email"`
		} `json:"referred"`
	}
	json.Unmarshal(ev.Data, &s)
	switch ev.Type {
	case "signup.created":
		return "New waitlist signup: " + s.Email
	case "signup.confirmed":
		return "Confirmed: " + s.Email
	case "signup.referred":
		if s.Referrer != nil && s.Referred != nil {
			return s.Referrer.Email + " referred " + s.Referred.Email
		}
	case "signup.invited":
		if s.Signup != nil {
			return "Invited: " + s.Signup.Email
		}
	case "signup.joined":
		return "Joined: " + s.Email
	case "signup.removed":
		return "Removed from waitlist: " + s.Email
	case "webhook.test":
		return "Lobby test event: your Slack webhook works."
	}
	return "Lobby event: " + ev.Type
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
