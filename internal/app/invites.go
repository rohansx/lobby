package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Invite struct {
	ID         int64  `json:"id"`
	SignupID   int64  `json:"signup_id"`
	Code       string `json:"code"`
	CreatedAt  int64  `json:"created_at"`
	ExpiresAt  *int64 `json:"expires_at"`
	RedeemedAt *int64 `json:"redeemed_at"`
	RevokedAt  *int64 `json:"revoked_at"`
}

// inviteToken is derived from the invite id rather than stored, so it can be
// re-sent from the dashboard without keeping a plaintext credential in the db.
func (a *App) inviteToken(inviteID int64) string {
	id := strconv.FormatInt(inviteID, 36)
	return id + "." + a.tokenSig("invite", id)
}

// InviteNext admits the first n people in line.
func (a *App) InviteNext(ctx context.Context, p *Project, n int) ([]*Signup, error) {
	if n <= 0 {
		return nil, errors.New("count must be positive")
	}
	ids, err := a.lineIDs(ctx, p, n)
	if err != nil {
		return nil, err
	}
	return a.InviteIDs(ctx, p, ids)
}

func (a *App) lineIDs(ctx context.Context, p *Project, n int) ([]int64, error) {
	rows, err := a.db.W.QueryContext(ctx, fmt.Sprintf(`SELECT id FROM signups WHERE project_id = ? AND status = ?
		ORDER BY %s, points DESC, base_position LIMIT ?`, rankExpr(p)), p.ID, StatusConfirmed, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// InviteIDs invites specific people. Anyone already invited, joined, removed
// or still unconfirmed is skipped.
func (a *App) InviteIDs(ctx context.Context, p *Project, ids []int64) ([]*Signup, error) {
	if err := a.checkBulkEmail(ctx, p, int64(len(ids))); err != nil {
		return nil, err
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var out []*Signup
	for _, id := range ids {
		s, err := getSignup(ctx, tx, p.ID, id)
		if errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		if s.Status != StatusConfirmed {
			continue
		}
		code := strings.ToUpper(randCode(4) + "-" + randCode(4))
		res, err := tx.ExecContext(ctx, `INSERT INTO invites (signup_id, token_hash, code, created_at) VALUES (?, ?, ?, ?)`,
			s.ID, "pending:"+randHex(8), code, now())
		if err != nil {
			return nil, err
		}
		inviteID, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx, `UPDATE invites SET token_hash = ? WHERE id = ?`, hashKey(a.inviteToken(inviteID)), inviteID); err != nil {
			return nil, err
		}
		t := now()
		if _, err := tx.ExecContext(ctx, `UPDATE signups SET status = ?, invited_at = ? WHERE id = ?`, StatusInvited, t, s.ID); err != nil {
			return nil, err
		}
		s.Status, s.InvitedAt = StatusInvited, &t
		if err := a.emit(ctx, tx, p.ID, "signup.invited", map[string]any{"signup": s, "invite_code": code}); err != nil {
			return nil, err
		}
		if err := a.enqueue(ctx, tx, "email.invite", emailJob{ProjectID: p.ID, SignupID: s.ID, InviteID: inviteID}); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	a.kick()
	return out, nil
}

func (a *App) LatestInvite(ctx context.Context, signupID int64) (*Invite, error) {
	inv := &Invite{}
	err := a.db.W.QueryRowContext(ctx, `SELECT id, signup_id, code, created_at, expires_at, redeemed_at, revoked_at
		FROM invites WHERE signup_id = ? AND revoked_at IS NULL ORDER BY id DESC LIMIT 1`, signupID).
		Scan(&inv.ID, &inv.SignupID, &inv.Code, &inv.CreatedAt, &inv.ExpiresAt, &inv.RedeemedAt, &inv.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return inv, err
}

// InviteLink is where the invite email points: the project's own signup page
// with ?wl=<token> when one is configured, else the person's status page.
func (a *App) InviteLink(p *Project, s *Signup, inviteID int64) string {
	if p.InviteURL != "" {
		sep := "?"
		if strings.Contains(p.InviteURL, "?") {
			sep = "&"
		}
		return p.InviteURL + sep + "wl=" + a.inviteToken(inviteID)
	}
	return a.cfg.BaseURL + "/s/" + a.StatusToken(s)
}

var ErrInvalidInvite = errors.New("invite is invalid, revoked or already used")

// Redeem marks an invite used and its signup joined. It accepts either the
// token from the invite link, or an email plus the code bound to it.
func (a *App) Redeem(ctx context.Context, p *Project, token, email, code string) (*Signup, error) {
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var inviteID, signupID int64
	if token != "" {
		err = tx.QueryRowContext(ctx, `SELECT i.id, i.signup_id FROM invites i JOIN signups s ON s.id = i.signup_id
			WHERE i.token_hash = ? AND s.project_id = ? AND i.revoked_at IS NULL AND i.redeemed_at IS NULL`,
			hashKey(token), p.ID).Scan(&inviteID, &signupID)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT i.id, i.signup_id FROM invites i JOIN signups s ON s.id = i.signup_id
			WHERE s.project_id = ? AND s.email_norm = ? AND i.code = ? AND i.revoked_at IS NULL AND i.redeemed_at IS NULL`,
			p.ID, normalizeEmail(email), strings.ToUpper(strings.TrimSpace(code))).Scan(&inviteID, &signupID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidInvite
	} else if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invites SET redeemed_at = ? WHERE id = ?`, now(), inviteID); err != nil {
		return nil, err
	}
	s, err := a.markJoined(ctx, tx, p, signupID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	a.kick()
	return s, nil
}

// MarkJoined is the call a product makes when a waitlisted person signs up.
func (a *App) MarkJoined(ctx context.Context, p *Project, id int64) (*Signup, error) {
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	s, err := a.markJoined(ctx, tx, p, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	a.kick()
	return s, nil
}

func (a *App) markJoined(ctx context.Context, tx *sql.Tx, p *Project, id int64) (*Signup, error) {
	s, err := getSignup(ctx, tx, p.ID, id)
	if err != nil {
		return nil, err
	}
	if s.Status == StatusJoined || s.Status == StatusRemoved {
		return s, nil
	}
	t := now()
	if _, err := tx.ExecContext(ctx, `UPDATE signups SET status = ?, joined_at = ? WHERE id = ?`, StatusJoined, t, id); err != nil {
		return nil, err
	}
	s.Status, s.JoinedAt = StatusJoined, &t
	return s, a.emit(ctx, tx, p.ID, "signup.joined", s)
}

type Stats struct {
	Total       int64            `json:"total"`
	ByStatus    map[string]int64 `json:"by_status"`
	ConfirmRate float64          `json:"confirm_rate"`
	Daily       []DayCount       `json:"daily"`
	TopSources  []NameCount      `json:"top_sources"`
	TopReferrer []ReferrerCount  `json:"top_referrers"`
	Referred    int64            `json:"referred"`
}

type DayCount struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type ReferrerCount struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Count int64  `json:"count"`
}

func (a *App) Stats(ctx context.Context, p *Project) (*Stats, error) {
	st := &Stats{ByStatus: map[string]int64{}}
	q := a.db.R
	rows, err := q.QueryContext(ctx, `SELECT status, COUNT(*) FROM signups WHERE project_id = ? GROUP BY status`, p.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s string
		var n int64
		rows.Scan(&s, &n)
		st.ByStatus[s] = n
		st.Total += n
	}
	rows.Close()
	everConfirmed := st.ByStatus[StatusConfirmed] + st.ByStatus[StatusInvited] + st.ByStatus[StatusJoined]
	if d := everConfirmed + st.ByStatus[StatusPending]; d > 0 {
		st.ConfirmRate = float64(everConfirmed) / float64(d)
	}

	rows, err = q.QueryContext(ctx, `SELECT date(created_at, 'unixepoch') d, COUNT(*) FROM signups
		WHERE project_id = ? AND created_at > ? GROUP BY d ORDER BY d`, p.ID, now()-30*86400)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d DayCount
		rows.Scan(&d.Day, &d.Count)
		st.Daily = append(st.Daily, d)
	}
	rows.Close()

	// A source is the utm_source when present, else the referring host, else "direct".
	rows, err = q.QueryContext(ctx, `SELECT src, COUNT(*) c FROM (
			SELECT CASE WHEN utm_source != '' THEN utm_source
				WHEN referrer != '' THEN
					CASE WHEN instr(substr(referrer, instr(referrer, '://') + 3), '/') > 0
						THEN substr(substr(referrer, instr(referrer, '://') + 3), 1, instr(substr(referrer, instr(referrer, '://') + 3), '/') - 1)
						ELSE substr(referrer, instr(referrer, '://') + 3) END
				WHEN referred_by IS NOT NULL THEN 'referral'
				ELSE 'direct' END src
			FROM signups WHERE project_id = ?)
		GROUP BY src ORDER BY c DESC LIMIT 10`, p.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var n NameCount
		rows.Scan(&n.Name, &n.Count)
		st.TopSources = append(st.TopSources, n)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx, `SELECT r.id, r.email, COUNT(*) c FROM signups s JOIN signups r ON r.id = s.referred_by
		WHERE s.project_id = ? AND s.referral_credited = 1 GROUP BY r.id ORDER BY c DESC LIMIT 10`, p.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r ReferrerCount
		rows.Scan(&r.ID, &r.Email, &r.Count)
		st.TopReferrer = append(st.TopReferrer, r)
	}
	rows.Close()
	q.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups WHERE project_id = ? AND referral_credited = 1`, p.ID).Scan(&st.Referred)
	return st, nil
}
