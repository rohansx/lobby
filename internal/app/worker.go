package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type emailJob struct {
	ProjectID   int64 `json:"project_id"`
	SignupID    int64 `json:"signup_id"`
	InviteID    int64 `json:"invite_id,omitempty"`
	BroadcastID int64 `json:"broadcast_id,omitempty"`
}

func (a *App) enqueue(ctx context.Context, tx *sql.Tx, kind string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs (kind, payload, run_at, created_at) VALUES (?, ?, ?, ?)`, kind, string(b), now(), now())
	return err
}

var jobBackoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 1 * time.Hour, 6 * time.Hour}

// RunWorker works jobs and webhook deliveries until ctx is cancelled. It runs
// in-process; everything it needs is in the same SQLite file.
func (a *App) RunWorker(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	lastMaint := time.Time{}
	for {
		for {
			n, err := a.runJobs(ctx)
			if err != nil {
				a.log.Error("jobs", "err", err)
			}
			m, err := a.deliverDue(ctx)
			if err != nil {
				a.log.Error("webhooks", "err", err)
			}
			if n+m == 0 || ctx.Err() != nil {
				break
			}
		}
		if time.Since(lastMaint) > time.Hour {
			a.maintain(ctx)
			lastMaint = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-a.wake:
		}
	}
}

func (a *App) runJobs(ctx context.Context) (int, error) {
	rows, err := a.db.W.QueryContext(ctx, `SELECT id, kind, payload, attempts FROM jobs WHERE status = 'queued' AND run_at <= ? ORDER BY run_at, id LIMIT 20`, now())
	if err != nil {
		return 0, err
	}
	type job struct {
		id       int64
		kind     string
		payload  string
		attempts int
	}
	var batch []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.kind, &j.payload, &j.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, j)
	}
	rows.Close()
	for _, j := range batch {
		if ctx.Err() != nil {
			break
		}
		err := a.runJob(ctx, j.kind, j.payload)
		attempts := j.attempts + 1
		switch {
		case err == nil:
			a.db.W.ExecContext(ctx, `UPDATE jobs SET status = 'done', attempts = ?, last_error = '' WHERE id = ?`, attempts, j.id)
		case errors.Is(err, ErrPermanent) || attempts > len(jobBackoff):
			a.log.Error("job failed", "kind", j.kind, "err", err)
			a.db.W.ExecContext(ctx, `UPDATE jobs SET status = 'failed', attempts = ?, last_error = ? WHERE id = ?`, attempts, clip(err.Error(), 500), j.id)
		default:
			a.log.Warn("job will retry", "kind", j.kind, "err", err)
			a.db.W.ExecContext(ctx, `UPDATE jobs SET attempts = ?, last_error = ?, run_at = ? WHERE id = ?`,
				attempts, clip(err.Error(), 500), now()+int64(jobBackoff[attempts-1].Seconds()), j.id)
		}
		// Resend allows 10 requests per second on every plan.
		if strings.HasPrefix(j.kind, "email.") {
			time.Sleep(110 * time.Millisecond)
		}
	}
	return len(batch), nil
}

func (a *App) runJob(ctx context.Context, kind, payload string) error {
	var j emailJob
	if err := json.Unmarshal([]byte(payload), &j); err != nil {
		return fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	p, err := a.ProjectByID(ctx, j.ProjectID)
	if errors.Is(err, ErrNotFound) {
		return nil // project deleted since
	} else if err != nil {
		return err
	}
	s, err := getSignup(ctx, a.db.W, p.ID, j.SignupID)
	if errors.Is(err, ErrNotFound) {
		return nil // signup deleted since
	} else if err != nil {
		return err
	}
	if s.Status == StatusRemoved || a.isSuppressed(ctx, p.ID, s.Email) {
		return nil
	}
	var m *Message
	switch kind {
	case "email.confirm":
		if s.Status != StatusPending {
			return nil
		}
		m = a.composeConfirm(p, s)
	case "email.status":
		m = a.composeStatus(p, s)
	case "email.invite":
		inv := &Invite{}
		err := a.db.W.QueryRowContext(ctx, `SELECT id, code FROM invites WHERE id = ? AND signup_id = ? AND revoked_at IS NULL`, j.InviteID, s.ID).
			Scan(&inv.ID, &inv.Code)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		m = a.composeInvite(p, s, inv)
	case "email.broadcast":
		if s.UnsubscribedAt != nil {
			return nil
		}
		var subject, body string
		if err := a.db.W.QueryRowContext(ctx, `SELECT subject, body FROM broadcasts WHERE id = ?`, j.BroadcastID).Scan(&subject, &body); err != nil {
			return nil
		}
		m = a.composeBroadcast(p, s, subject, body)
	default:
		return fmt.Errorf("%w: unknown job kind %q", ErrPermanent, kind)
	}
	return a.mailer.Send(ctx, m)
}

// Broadcast queues one email per recipient. It is the single launch email the
// research says every waitlist needs; anything more belongs in a newsletter tool.
func (a *App) Broadcast(ctx context.Context, p *Project, subject, body, audience string) (int, error) {
	subject, body = strings.TrimSpace(subject), strings.TrimSpace(body)
	if subject == "" || body == "" {
		return 0, errors.New("subject and body are required")
	}
	statuses := map[string][]any{
		"confirmed": {StatusConfirmed},
		"invited":   {StatusInvited},
		"all":       {StatusConfirmed, StatusInvited, StatusJoined},
	}[audience]
	if statuses == nil {
		return 0, errors.New("audience must be confirmed, invited or all")
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO broadcasts (project_id, subject, body, audience, created_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, subject, body, audience, now())
	if err != nil {
		return 0, err
	}
	bid, _ := res.LastInsertId()
	ph := strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")
	rows, err := tx.QueryContext(ctx, `SELECT id FROM signups WHERE project_id = ? AND unsubscribed_at IS NULL AND status IN (`+ph+`)`,
		append([]any{p.ID}, statuses...)...)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := a.enqueue(ctx, tx, "email.broadcast", emailJob{ProjectID: p.ID, SignupID: id, BroadcastID: bid}); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE broadcasts SET recipients = ? WHERE id = ?`, len(ids), bid); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	a.kick()
	return len(ids), nil
}

type Broadcast struct {
	ID         int64  `json:"id"`
	Subject    string `json:"subject"`
	Audience   string `json:"audience"`
	Recipients int    `json:"recipients"`
	CreatedAt  int64  `json:"created_at"`
}

func (a *App) Broadcasts(ctx context.Context, p *Project) ([]Broadcast, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT id, subject, audience, recipients, created_at FROM broadcasts WHERE project_id = ? ORDER BY id DESC`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Broadcast
	for rows.Next() {
		var b Broadcast
		rows.Scan(&b.ID, &b.Subject, &b.Audience, &b.Recipients, &b.CreatedAt)
		out = append(out, b)
	}
	return out, rows.Err()
}

// QueueStats reports outstanding and failed email jobs for the dashboard.
func (a *App) QueueStats(ctx context.Context) (queued, failed int) {
	a.db.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status = 'queued'`).Scan(&queued)
	a.db.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status = 'failed'`).Scan(&failed)
	return
}

// maintain clears IP hashes older than 30 days, expired sessions and old
// finished jobs and deliveries.
func (a *App) maintain(ctx context.Context) {
	cutoff := now() - 30*86400
	a.db.W.ExecContext(ctx, `UPDATE signups SET ip_hmac = '' WHERE ip_hmac != '' AND created_at < ?`, cutoff)
	a.db.W.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now())
	a.db.W.ExecContext(ctx, `DELETE FROM jobs WHERE status = 'done' AND created_at < ?`, cutoff)
	a.db.W.ExecContext(ctx, `DELETE FROM deliveries WHERE status IN ('success', 'skipped') AND created_at < ?`, now()-90*86400)
}
