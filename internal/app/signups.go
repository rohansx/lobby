package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusInvited   = "invited"
	StatusJoined    = "joined"
	StatusRemoved   = "removed"
)

type Signup struct {
	ID               int64  `json:"id"`
	ProjectID        int64  `json:"-"`
	Email            string `json:"email"`
	Status           string `json:"status"`
	BasePosition     int64  `json:"base_position"`
	Points           int64  `json:"points"`
	ReferralCode     string `json:"referral_code"`
	ReferredBy       *int64 `json:"referred_by"`
	ReferralCredited bool   `json:"referral_credited"`
	SourceURL        string `json:"source_url"`
	Referrer         string `json:"referrer"`
	UTMSource        string `json:"utm_source"`
	UTMMedium        string `json:"utm_medium"`
	UTMCampaign      string `json:"utm_campaign"`
	UTMTerm          string `json:"utm_term"`
	UTMContent       string `json:"utm_content"`
	ConsentVersion   int    `json:"consent_version"`
	IPHMAC           string `json:"-"`
	CreatedAt        int64  `json:"created_at"`
	ConfirmedAt      *int64 `json:"confirmed_at"`
	InvitedAt        *int64 `json:"invited_at"`
	JoinedAt         *int64 `json:"joined_at"`
	RemovedAt        *int64 `json:"removed_at"`
	UnsubscribedAt   *int64 `json:"unsubscribed_at"`
	LastStatusEmail  *int64 `json:"-"`
}

const signupCols = `id, project_id, email, status, base_position, points, referral_code, referred_by, referral_credited,
	source_url, referrer, utm_source, utm_medium, utm_campaign, utm_term, utm_content, consent_version, ip_hmac,
	created_at, confirmed_at, invited_at, joined_at, removed_at, unsubscribed_at, last_status_email_at`

func scanSignup(row scanner) (*Signup, error) {
	s := &Signup{}
	err := row.Scan(&s.ID, &s.ProjectID, &s.Email, &s.Status, &s.BasePosition, &s.Points, &s.ReferralCode,
		&s.ReferredBy, &s.ReferralCredited, &s.SourceURL, &s.Referrer, &s.UTMSource, &s.UTMMedium, &s.UTMCampaign,
		&s.UTMTerm, &s.UTMContent, &s.ConsentVersion, &s.IPHMAC, &s.CreatedAt, &s.ConfirmedAt, &s.InvitedAt,
		&s.JoinedAt, &s.RemovedAt, &s.UnsubscribedAt, &s.LastStatusEmail)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

type querier interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func getSignup(ctx context.Context, q querier, projectID, id int64) (*Signup, error) {
	return scanSignup(q.QueryRowContext(ctx, `SELECT `+signupCols+` FROM signups WHERE project_id = ? AND id = ?`, projectID, id))
}

func (a *App) Signup(ctx context.Context, projectID, id int64) (*Signup, error) {
	return getSignup(ctx, a.db.W, projectID, id)
}

// SignupByEmail looks up by normalized address.
func (a *App) SignupByEmail(ctx context.Context, projectID int64, email string) (*Signup, error) {
	return scanSignup(a.db.W.QueryRowContext(ctx, `SELECT `+signupCols+` FROM signups WHERE project_id = ? AND email_norm = ?`,
		projectID, normalizeEmail(email)))
}

type SignupInput struct {
	Email       string
	Ref         string
	SourceURL   string
	Referrer    string
	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMTerm     string
	UTMContent  string
	IP          string
	Honeypot    string
	DwellMs     int64 // milliseconds between form render and submit; -1 when unknown
}

type SignupResult struct {
	Signup  *Signup
	Created bool // false when the address was already on the list
	Fake    bool // a spam check tripped; the caller must answer exactly as for a real signup
}

// ValidationError carries a message that is safe to show the visitor.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

const minDwellMs = 1500

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// SubmitSignup is the hot path: validate, one transaction that inserts the
// signup and enqueues its email and webhooks, then wake the worker.
func (a *App) SubmitSignup(ctx context.Context, p *Project, in SignupInput) (*SignupResult, error) {
	email, domain, err := parseEmail(in.Email)
	if err != nil {
		return nil, &ValidationError{err.Error()}
	}
	if in.Honeypot != "" || (in.DwellMs >= 0 && in.DwellMs < minDwellMs) {
		return &SignupResult{Fake: true}, nil
	}
	if a.spam.IsDisposable(domain) {
		return nil, &ValidationError{"Please use a permanent email address."}
	}
	if !a.spam.HasMX(ctx, domain) {
		return nil, &ValidationError{"That email domain can't receive mail. Check for a typo?"}
	}
	if a.isSuppressed(ctx, p.ID, email) {
		return &SignupResult{Fake: true}, nil
	}

	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	norm := normalizeEmail(email)
	existing, err := scanSignup(tx.QueryRowContext(ctx, `SELECT `+signupCols+` FROM signups WHERE project_id = ? AND email_norm = ?`, p.ID, norm))
	if err == nil {
		if existing.Status == StatusRemoved {
			return &SignupResult{Fake: true}, nil
		}
		return &SignupResult{Signup: existing}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	var referredBy *int64
	if code := strings.ToLower(strings.TrimSpace(in.Ref)); code != "" {
		var rid int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM signups WHERE project_id = ? AND referral_code = ? AND status != ?`,
			p.ID, code, StatusRemoved).Scan(&rid)
		if err == nil {
			referredBy = &rid
		}
	}

	status := StatusPending
	var confirmedAt *int64
	if !p.DOIEnabled {
		status = StatusConfirmed
		t := now()
		confirmedAt = &t
	}
	var base int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(base_position), 0) + 1 FROM signups WHERE project_id = ?`, p.ID).Scan(&base); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO signups (project_id, email, email_norm, status, base_position, referral_code,
		referred_by, source_url, referrer, utm_source, utm_medium, utm_campaign, utm_term, utm_content, consent_version,
		ip_hmac, created_at, confirmed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, email, norm, status, base, randCode(8), referredBy, clip(in.SourceURL, 1000), clip(in.Referrer, 1000),
		clip(in.UTMSource, 200), clip(in.UTMMedium, 200), clip(in.UTMCampaign, 200), clip(in.UTMTerm, 200),
		clip(in.UTMContent, 200), p.ConsentVersion, a.ipHMAC(in.IP), now(), confirmedAt)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	s, err := getSignup(ctx, tx, p.ID, id)
	if err != nil {
		return nil, err
	}
	if err := a.emit(ctx, tx, p.ID, "signup.created", s); err != nil {
		return nil, err
	}
	if p.DOIEnabled {
		err = a.enqueue(ctx, tx, "email.confirm", emailJob{ProjectID: p.ID, SignupID: id})
	} else {
		if err := a.creditReferral(ctx, tx, p, s); err != nil {
			return nil, err
		}
		err = a.enqueue(ctx, tx, "email.status", emailJob{ProjectID: p.ID, SignupID: id})
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	a.kick()
	return &SignupResult{Signup: s, Created: true}, nil
}

// ResendStatus emails an existing signup their link again, at most once per 10 minutes.
func (a *App) ResendStatus(ctx context.Context, p *Project, s *Signup) error {
	if s.LastStatusEmail != nil && now()-*s.LastStatusEmail < 600 {
		return nil
	}
	kind := "email.status"
	if s.Status == StatusPending {
		kind = "email.confirm"
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE signups SET last_status_email_at = ? WHERE id = ?`, now(), s.ID); err != nil {
		return err
	}
	if err := a.enqueue(ctx, tx, kind, emailJob{ProjectID: p.ID, SignupID: s.ID}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.kick()
	return nil
}

// Confirm marks a pending signup confirmed, which is also when its referrer gets credit.
func (a *App) Confirm(ctx context.Context, p *Project, id int64) (*Signup, bool, error) {
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	s, err := getSignup(ctx, tx, p.ID, id)
	if err != nil {
		return nil, false, err
	}
	if s.Status != StatusPending {
		return s, false, nil
	}
	t := now()
	if _, err := tx.ExecContext(ctx, `UPDATE signups SET status = ?, confirmed_at = ? WHERE id = ?`, StatusConfirmed, t, id); err != nil {
		return nil, false, err
	}
	s.Status, s.ConfirmedAt = StatusConfirmed, &t
	if err := a.creditReferral(ctx, tx, p, s); err != nil {
		return nil, false, err
	}
	if err := a.emit(ctx, tx, p.ID, "signup.confirmed", s); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	a.kick()
	return s, true, nil
}

// creditReferral gives the referrer points once the referred person is
// confirmed, unless that would break the per-IP or per-hour caps.
func (a *App) creditReferral(ctx context.Context, tx *sql.Tx, p *Project, s *Signup) error {
	if s.ReferredBy == nil || s.ReferralCredited {
		return nil
	}
	ref, err := getSignup(ctx, tx, p.ID, *s.ReferredBy)
	if errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if ref.Status == StatusRemoved || ref.Status == StatusPending {
		return nil
	}
	if s.IPHMAC != "" && p.ReferralIPCap > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups WHERE referred_by = ? AND referral_credited = 1 AND ip_hmac = ?`,
			ref.ID, s.IPHMAC).Scan(&n); err != nil {
			return err
		}
		if n >= p.ReferralIPCap {
			return nil
		}
	}
	if p.ReferralHourlyCap > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups WHERE referred_by = ? AND referral_credited = 1 AND confirmed_at > ?`,
			ref.ID, now()-3600).Scan(&n); err != nil {
			return err
		}
		if n >= p.ReferralHourlyCap {
			return nil
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE signups SET referral_credited = 1 WHERE id = ?`, s.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE signups SET points = points + ? WHERE id = ?`, p.PointsPerReferral, ref.ID); err != nil {
		return err
	}
	s.ReferralCredited = true
	ref.Points += int64(p.PointsPerReferral)
	return a.emit(ctx, tx, p.ID, "signup.referred", map[string]any{"referrer": ref, "referred": s})
}

// Place describes where a confirmed signup stands in line.
type Place struct {
	Position  int64 `json:"position"`  // 1-based, among people still waiting
	Behind    int64 `json:"behind"`    // people waiting behind them
	Total     int64 `json:"total"`     // people waiting
	Referrals int64 `json:"referrals"` // credited referrals
}

// rankExpr orders the line: base position minus referral points in points
// mode (each credited referral moves you up that many places), plain FIFO otherwise.
func rankExpr(p *Project) string {
	if p.PositionMode == "points" {
		return "(base_position - points)"
	}
	return "base_position"
}

func (a *App) PlaceOf(ctx context.Context, p *Project, s *Signup) (Place, error) {
	var pl Place
	q := a.db.R
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups WHERE referred_by = ? AND referral_credited = 1`, s.ID).Scan(&pl.Referrals); err != nil {
		return pl, err
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups WHERE project_id = ? AND status = ?`, p.ID, StatusConfirmed).Scan(&pl.Total); err != nil {
		return pl, err
	}
	if s.Status != StatusConfirmed {
		return pl, nil
	}
	rank := rankExpr(p)
	myRank := s.BasePosition
	if p.PositionMode == "points" {
		myRank -= s.Points
	}
	// Ties go to the person with more points, so a referral moves someone up
	// exactly points_per_referral places rather than one fewer.
	var ahead int64
	if err := q.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM signups WHERE project_id = ? AND status = ?
		AND (%[1]s < ? OR (%[1]s = ? AND (points > ? OR (points = ? AND base_position < ?))))`, rank),
		p.ID, StatusConfirmed, myRank, myRank, s.Points, s.Points, s.BasePosition).Scan(&ahead); err != nil {
		return pl, err
	}
	pl.Position = ahead + 1
	pl.Behind = pl.Total - pl.Position
	return pl, nil
}

// PublicCount is the confirmed-plus-admitted count, or -1 below the project's floor.
func (a *App) PublicCount(ctx context.Context, p *Project) (int64, error) {
	var n int64
	err := a.db.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups WHERE project_id = ? AND status IN (?, ?, ?)`,
		p.ID, StatusConfirmed, StatusInvited, StatusJoined).Scan(&n)
	if err != nil {
		return 0, err
	}
	if n < int64(p.CounterFloor) {
		return -1, nil
	}
	return n, nil
}

// StatusToken is a signed, stable token for a signup's status page. Lookups
// never take a raw email, so the public API is not an enumeration oracle.
func (a *App) StatusToken(s *Signup) string {
	id := strconv.FormatInt(s.ID, 36)
	return id + "." + a.tokenSig("status", id)
}

func (a *App) tokenSig(purpose, id string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(purpose + ":" + id))
	return hex.EncodeToString(m.Sum(nil))[:24]
}

func (a *App) parseToken(purpose, tok string) (int64, bool) {
	id, sig, ok := strings.Cut(tok, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(a.tokenSig(purpose, id))) {
		return 0, false
	}
	n, err := strconv.ParseInt(id, 36, 64)
	return n, err == nil
}

func (a *App) SignupByStatusToken(ctx context.Context, tok string) (*Signup, *Project, error) {
	id, ok := a.parseToken("status", tok)
	if !ok {
		return nil, nil, ErrNotFound
	}
	s, err := scanSignup(a.db.W.QueryRowContext(ctx, `SELECT `+signupCols+` FROM signups WHERE id = ?`, id))
	if err != nil {
		return nil, nil, err
	}
	p, err := a.ProjectByID(ctx, s.ProjectID)
	return s, p, err
}

func (a *App) UnsubscribeToken(s *Signup) string {
	id := strconv.FormatInt(s.ID, 36)
	return id + "." + a.tokenSig("unsub", id)
}

func (a *App) Unsubscribe(ctx context.Context, tok string) error {
	id, ok := a.parseToken("unsub", tok)
	if !ok {
		return ErrNotFound
	}
	_, err := a.db.W.ExecContext(ctx, `UPDATE signups SET unsubscribed_at = COALESCE(unsubscribed_at, ?) WHERE id = ?`, now(), id)
	return err
}

// Remove takes someone off the list. A hard delete also writes a suppression
// entry so a bot cannot re-add the address.
func (a *App) Remove(ctx context.Context, p *Project, id int64, hard bool) error {
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	s, err := getSignup(ctx, tx, p.ID, id)
	if err != nil {
		return err
	}
	if hard {
		if _, err := tx.ExecContext(ctx, `DELETE FROM signups WHERE id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO suppressions (project_id, email_hmac, reason, created_at) VALUES (?, ?, 'deleted', ?)`,
			p.ID, a.emailHMAC(s.Email), now()); err != nil {
			return err
		}
		if err := a.emit(ctx, tx, p.ID, "signup.removed", map[string]any{"id": s.ID, "deleted": true}); err != nil {
			return err
		}
	} else {
		if s.Status == StatusRemoved {
			return nil
		}
		t := now()
		if _, err := tx.ExecContext(ctx, `UPDATE signups SET status = ?, removed_at = ? WHERE id = ?`, StatusRemoved, t, id); err != nil {
			return err
		}
		s.Status, s.RemovedAt = StatusRemoved, &t
		if err := a.emit(ctx, tx, p.ID, "signup.removed", s); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.kick()
	return nil
}

func (a *App) isSuppressed(ctx context.Context, projectID int64, email string) bool {
	var n int
	a.db.W.QueryRowContext(ctx, `SELECT COUNT(*) FROM suppressions WHERE project_id IN (0, ?) AND email_hmac = ?`,
		projectID, a.emailHMAC(email)).Scan(&n)
	return n > 0
}

func (a *App) Suppress(ctx context.Context, email, reason string) error {
	_, err := a.db.W.ExecContext(ctx, `INSERT OR IGNORE INTO suppressions (project_id, email_hmac, reason, created_at) VALUES (0, ?, ?, ?)`,
		a.emailHMAC(email), reason, now())
	return err
}

type SignupFilter struct {
	Status string
	Query  string
	Before int64 // cursor: return ids lower than this
	Limit  int
}

func (a *App) ListSignups(ctx context.Context, p *Project, f SignupFilter) ([]*Signup, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	q := `SELECT ` + signupCols + ` FROM signups WHERE project_id = ?`
	args := []any{p.ID}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.Query != "" {
		q += ` AND (email LIKE ? OR utm_source LIKE ? OR referrer LIKE ?)`
		like := "%" + f.Query + "%"
		args = append(args, like, like, like)
	}
	if f.Before > 0 {
		q += ` AND id < ?`
		args = append(args, f.Before)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, f.Limit)
	return a.querySignups(ctx, q, args...)
}

func (a *App) querySignups(ctx context.Context, q string, args ...any) ([]*Signup, error) {
	rows, err := a.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Signup
	for rows.Next() {
		s, err := scanSignup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// EachSignup streams signups for export in id order.
func (a *App) EachSignup(ctx context.Context, p *Project, status string, fn func(*Signup) error) error {
	q := `SELECT ` + signupCols + ` FROM signups WHERE project_id = ?`
	args := []any{p.ID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	rows, err := a.db.R.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		s, err := scanSignup(rows)
		if err != nil {
			return err
		}
		if err := fn(s); err != nil {
			return err
		}
	}
	return rows.Err()
}
