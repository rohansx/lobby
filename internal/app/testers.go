package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Testers are people the founder adds by email so they can send feedback,
// without joining the waitlist themselves: beta users, friends, a client.
// They're stored as joined signups with feedback switched on, and as never
// subscribed to waitlist mail, so the only email they get from Lobby is the
// reply to their own reports. Nothing is sent when they're added.

const (
	maxTestersPerAdd = 200
	testerSource     = "tester" // shows as the signup's source
)

type TesterResult struct {
	Added   int      `json:"added"`   // new people
	Updated int      `json:"updated"` // already on the list; feedback switched on
	Skipped []string `json:"skipped"` // invalid, removed or blocked addresses, with why
}

// AddTesters switches feedback on for each address, adding anyone not on
// the list yet.
func (a *App) AddTesters(ctx context.Context, p *Project, raw []string) (*TesterResult, error) {
	res := &TesterResult{Skipped: []string{}}
	seen := map[string]bool{}
	var emails []string
	for _, e := range raw {
		e = strings.Trim(strings.TrimSpace(e), "<>,;")
		if e == "" {
			continue
		}
		email, _, err := parseEmail(e)
		if err != nil {
			res.Skipped = append(res.Skipped, e+": not a valid email address")
			continue
		}
		if norm := normalizeEmail(email); !seen[norm] {
			seen[norm] = true
			emails = append(emails, email)
		}
	}
	if len(emails) > maxTestersPerAdd {
		return nil, &ValidationError{fmt.Sprintf("Add up to %d people at a time.", maxTestersPerAdd)}
	}
	plan, owner := a.planForOwner(ctx, p.OwnerID)
	if owner != nil && owner.Suspended {
		return nil, ErrWaitlistFull
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t := now()
	for _, email := range emails {
		norm := normalizeEmail(email)
		existing, err := scanSignup(tx.QueryRowContext(ctx, `SELECT `+signupCols+` FROM signups WHERE project_id = ? AND email_norm = ?`, p.ID, norm))
		switch {
		case err == nil && existing.Status == StatusRemoved:
			res.Skipped = append(res.Skipped, email+": removed from this waitlist")
			continue
		case err == nil:
			if _, err := tx.ExecContext(ctx, `UPDATE signups SET feedback_access = 1 WHERE id = ?`, existing.ID); err != nil {
				return nil, err
			}
			res.Updated++
			continue
		case !errors.Is(err, ErrNotFound):
			return nil, err
		}
		var suppressed int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM suppressions WHERE project_id IN (0, ?) AND email_hmac = ?`,
			p.ID, a.emailHMAC(email)).Scan(&suppressed); err != nil {
			return nil, err
		}
		if suppressed > 0 { // inside the transaction: the writer pool has one connection
			res.Skipped = append(res.Skipped, email+": deleted earlier or bounced")
			continue
		}
		if plan.Signups >= 0 && a.signupsUsed(ctx, tx, p.OwnerID) >= plan.Signups {
			res.Skipped = append(res.Skipped, email+": the plan's signup limit is reached")
			continue
		}
		var base int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(base_position), 0) + 1 FROM signups WHERE project_id = ?`, p.ID).Scan(&base); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO signups (project_id, email, email_norm, status, base_position, referral_code,
			utm_source, consent_version, created_at, confirmed_at, joined_at, unsubscribed_at, feedback_access)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, 1)`,
			p.ID, email, norm, StatusJoined, base, randCode(8), testerSource, t, t, t, t); err != nil {
			return nil, err
		}
		res.Added++
	}
	return res, tx.Commit()
}
