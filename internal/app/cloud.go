package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// Cloud mode: open signup, one account per admin row, projects scoped to
// their owner, and plan limits. A self-hosted instance (LOBBY_CLOUD unset)
// skips all of it: one admin, every project, no limits.

// Plan is what an account may do. -1 means unlimited.
type Plan struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Signups   int64  `json:"signups"`  // across all of the account's waitlists
	Projects  int64  `json:"projects"` // waitlists
	Emails    int64  `json:"emails"`   // per calendar month (UTC)
	Webhooks  bool   `json:"webhooks"` // webhooks and the secret-key REST API
	Branding  bool   `json:"branding"` // "powered by Lobby" on pages and emails
	Monthly   int    `json:"monthly"`  // price in USD
	YearlyPer int    `json:"yearly"`   // price per month, billed yearly
}

var plans = map[string]Plan{
	"free":   {ID: "free", Name: "Free", Signups: 500, Projects: 1, Emails: 1000, Branding: true},
	"launch": {ID: "launch", Name: "Launch", Signups: -1, Projects: 3, Emails: 5000, Webhooks: true, Monthly: 5, YearlyPer: 4},
	"pro":    {ID: "pro", Name: "Pro", Signups: -1, Projects: -1, Emails: 25000, Webhooks: true, Monthly: 12, YearlyPer: 10},
}

var selfHosted = Plan{ID: "self", Name: "Self-hosted", Signups: -1, Projects: -1, Emails: -1, Webhooks: true}

// ErrLimit is a plan limit; the dashboard shows its message with an upgrade link.
type ErrLimit struct{ Msg string }

func (e *ErrLimit) Error() string { return e.Msg }

// Account is an admin seen as a cloud customer.
type Account struct {
	ID                int64  `json:"id"`
	Email             string `json:"email"`
	Verified          bool   `json:"verified"`
	Plan              string `json:"plan"`
	Interval          string `json:"interval"`
	SubStatus         string `json:"status"`
	PeriodEnd         *int64 `json:"period_end"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
	PolarCustomerID   string `json:"-"`
	PolarSubscription string `json:"-"`
	LimitMailedAt     *int64 `json:"-"`
	CreatedAt         int64  `json:"created_at"`
	HasPassword       bool   `json:"has_password"`
}

const accountCols = `id, email, verified_at IS NOT NULL, plan, plan_interval, sub_status, period_end, cancel_at_period_end,
	polar_customer_id, polar_subscription_id, limit_mailed_at, created_at, password_hash != ''`

func scanAccount(row scanner) (*Account, error) {
	ac := &Account{}
	err := row.Scan(&ac.ID, &ac.Email, &ac.Verified, &ac.Plan, &ac.Interval, &ac.SubStatus, &ac.PeriodEnd,
		&ac.CancelAtPeriodEnd, &ac.PolarCustomerID, &ac.PolarSubscription, &ac.LimitMailedAt, &ac.CreatedAt, &ac.HasPassword)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return ac, err
}

func (a *App) Account(ctx context.Context, id int64) (*Account, error) {
	return scanAccount(a.db.W.QueryRowContext(ctx, `SELECT `+accountCols+` FROM admins WHERE id = ?`, id))
}

// scope is the owner filter for project queries: 0 (everything) when self-hosted.
func (a *App) scope(ownerID int64) int64 {
	if !a.cfg.Cloud {
		return 0
	}
	return ownerID
}

// PlanOf returns the limits that apply to an account right now.
func (a *App) PlanOf(ac *Account) Plan {
	if !a.cfg.Cloud {
		return selfHosted
	}
	if p, ok := plans[ac.Plan]; ok {
		return p
	}
	return plans["free"]
}

func (a *App) planForOwner(ctx context.Context, ownerID int64) (Plan, *Account) {
	if !a.cfg.Cloud {
		return selfHosted, nil
	}
	ac, err := a.Account(ctx, ownerID)
	if err != nil {
		return plans["free"], nil
	}
	return a.PlanOf(ac), ac
}

// Usage is what an account has used against its plan.
type Usage struct {
	Signups  int64 `json:"signups"`
	Projects int64 `json:"projects"`
	Emails   int64 `json:"emails"`
}

func month() string { return time.Now().UTC().Format("2006-01") }

func (a *App) UsageOf(ctx context.Context, ownerID int64) Usage {
	var u Usage
	a.db.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE owner_id = ?`, ownerID).Scan(&u.Projects)
	u.Signups = a.signupsUsed(ctx, a.db.R, ownerID)
	a.db.R.QueryRowContext(ctx, `SELECT COALESCE(SUM(sent), 0) FROM email_usage WHERE admin_id = ? AND month = ?`, ownerID, month()).Scan(&u.Emails)
	return u
}

func (a *App) signupsUsed(ctx context.Context, q querier, ownerID int64) int64 {
	var n int64
	q.QueryRowContext(ctx, `SELECT COUNT(*) FROM signups WHERE project_id IN (SELECT id FROM projects WHERE owner_id = ?) AND status != ?`,
		ownerID, StatusRemoved).Scan(&n)
	return n
}

// checkProjectLimit runs before a project is created.
func (a *App) checkProjectLimit(ctx context.Context, ownerID int64) error {
	plan, ac := a.planForOwner(ctx, ownerID)
	if ac != nil && !ac.Verified {
		return errors.New("Confirm your email address first. We sent you a link when you signed up.")
	}
	if plan.Projects < 0 {
		return nil
	}
	if a.UsageOf(ctx, ownerID).Projects >= plan.Projects {
		return &ErrLimit{fmt.Sprintf("The %s plan includes %s. Upgrade to add another.", plan.Name, plural(plan.Projects, "waitlist"))}
	}
	return nil
}

// signupAllowed runs inside the signup transaction, before the insert. The
// plan is looked up before the transaction opens, since the writer pool has
// one connection and the transaction holds it.
func (a *App) signupAllowed(ctx context.Context, tx *sql.Tx, p *Project, plan Plan, ac *Account) bool {
	if plan.Signups < 0 {
		return true
	}
	if a.signupsUsed(ctx, tx, p.OwnerID) < plan.Signups {
		return true
	}
	// Tell the owner once a week that signups are being turned away.
	if ac != nil && (ac.LimitMailedAt == nil || now()-*ac.LimitMailedAt > 7*86400) {
		tx.ExecContext(ctx, `UPDATE admins SET limit_mailed_at = ? WHERE id = ?`, now(), ac.ID)
		go a.mailLimitReached(context.WithoutCancel(ctx), ac, plan)
	}
	return false
}

// emailAllowance is how many more emails the owner can send this month (-1: no cap).
func (a *App) emailAllowance(ctx context.Context, ownerID int64) int64 {
	plan, _ := a.planForOwner(ctx, ownerID)
	if plan.Emails < 0 {
		return -1
	}
	left := plan.Emails - a.UsageOf(ctx, ownerID).Emails
	if left < 0 {
		left = 0
	}
	return left
}

// checkBulkEmail refuses a broadcast or invite batch that would go over the monthly cap.
func (a *App) checkBulkEmail(ctx context.Context, p *Project, n int64) error {
	left := a.emailAllowance(ctx, p.OwnerID)
	if left < 0 || n <= left {
		return nil
	}
	plan, _ := a.planForOwner(ctx, p.OwnerID)
	return &ErrLimit{fmt.Sprintf("That's %d emails, and your %s plan has %d left this month. Upgrade for a higher limit.", n, plan.Name, left)}
}

// countEmail records one sent email against the project owner's month.
func (a *App) countEmail(ctx context.Context, p *Project) {
	if !a.cfg.Cloud || p.OwnerID == 0 {
		return
	}
	a.db.W.ExecContext(ctx, `INSERT INTO email_usage (admin_id, month, sent) VALUES (?, ?, 1)
		ON CONFLICT(admin_id, month) DO UPDATE SET sent = sent + 1`, p.OwnerID, month())
}

// branded reports whether a project's pages and emails carry the Lobby badge.
func (a *App) branded(ctx context.Context, p *Project) bool {
	if !a.cfg.Cloud {
		return false
	}
	plan, _ := a.planForOwner(ctx, p.OwnerID)
	return plan.Branding
}

func plural(n int64, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// Accounts

const (
	tokenVerify = "verify"
	tokenReset  = "reset"
)

// SignUp creates a cloud account and returns its id.
func (a *App) SignUp(ctx context.Context, email, password string) (int64, error) {
	email = strings.TrimSpace(email)
	addr, domain, err := parseEmail(email)
	if err != nil {
		return 0, err
	}
	if a.spam.IsDisposable(domain) {
		return 0, errors.New("Please use a permanent email address.")
	}
	if len(password) < 10 {
		return 0, errors.New("Use a password of at least 10 characters.")
	}
	h, err := hashPassword(password)
	if err != nil {
		return 0, err
	}
	// The email column is case-sensitive, so check case-insensitively in the same statement.
	res, err := a.db.W.ExecContext(ctx, `INSERT INTO admins (email, password_hash, created_at)
		SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM admins WHERE email = ? COLLATE NOCASE)`, addr, h, now(), addr)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, errors.New("There's already an account with that email. Sign in instead.")
	}
	return res.LastInsertId()
}

func (a *App) newAuthToken(ctx context.Context, adminID int64, purpose string, ttl time.Duration) (string, error) {
	tok := randHex(24)
	_, err := a.db.W.ExecContext(ctx, `INSERT INTO auth_tokens (token_hash, admin_id, purpose, expires_at) VALUES (?, ?, ?, ?)`,
		hashKey(tok), adminID, purpose, now()+int64(ttl.Seconds()))
	return tok, err
}

// useAuthToken consumes a single-use token and returns its admin.
func (a *App) useAuthToken(ctx context.Context, tok, purpose string) (int64, error) {
	var id int64
	err := a.db.W.QueryRowContext(ctx, `DELETE FROM auth_tokens WHERE token_hash = ? AND purpose = ? AND expires_at > ? RETURNING admin_id`,
		hashKey(tok), purpose, now()).Scan(&id)
	if err != nil {
		return 0, errors.New("This link has expired or was already used.")
	}
	return id, nil
}

func (a *App) SendVerification(ctx context.Context, ac *Account) error {
	tok, err := a.newAuthToken(ctx, ac.ID, tokenVerify, 72*time.Hour)
	if err != nil {
		return err
	}
	text, htm := email{
		Project:   "Lobby",
		Preheader: "One click to confirm your Lobby account.",
		Heading:   "Confirm your email",
		Paras:     []string{"Thanks for signing up for Lobby. Confirm your address and you can create your first waitlist."},
		CTA:       "Confirm email",
		URL:       a.cfg.BaseURL + "/verify/" + tok,
		Footer:    "If you didn't create a Lobby account, ignore this email.",
	}.render()
	return a.mailer.Send(ctx, &Message{From: a.lobbyFrom(), To: ac.Email, Subject: "Confirm your Lobby account", Text: text, HTML: htm})
}

func (a *App) Verify(ctx context.Context, tok string) (int64, error) {
	id, err := a.useAuthToken(ctx, tok, tokenVerify)
	if err != nil {
		return 0, err
	}
	_, err = a.db.W.ExecContext(ctx, `UPDATE admins SET verified_at = COALESCE(verified_at, ?) WHERE id = ?`, now(), id)
	return id, err
}

// SendReset emails a password reset link. Unknown addresses get nothing, and
// the caller answers the same either way.
func (a *App) SendReset(ctx context.Context, addr string) error {
	var id int64
	var to string
	err := a.db.W.QueryRowContext(ctx, `SELECT id, email FROM admins WHERE email = ? COLLATE NOCASE`, strings.TrimSpace(addr)).Scan(&id, &to)
	if err != nil {
		return nil
	}
	tok, err := a.newAuthToken(ctx, id, tokenReset, time.Hour)
	if err != nil {
		return err
	}
	text, htm := email{
		Project:   "Lobby",
		Preheader: "Reset your Lobby password.",
		Heading:   "Reset your password",
		Paras:     []string{"Someone asked to reset the password for this Lobby account. The link works for one hour."},
		CTA:       "Choose a new password",
		URL:       a.cfg.BaseURL + "/reset?token=" + tok,
		Footer:    "If you didn't ask for this, ignore this email. Your password stays the same.",
	}.render()
	return a.mailer.Send(ctx, &Message{From: a.lobbyFrom(), To: to, Subject: "Reset your Lobby password", Text: text, HTML: htm})
}

// ResetPassword sets a new password, signs out every session and verifies the
// address (the link proved the person reads that inbox).
func (a *App) ResetPassword(ctx context.Context, tok, password string) (string, error) {
	if len(password) < 10 {
		return "", errors.New("Use a password of at least 10 characters.")
	}
	id, err := a.useAuthToken(ctx, tok, tokenReset)
	if err != nil {
		return "", err
	}
	h, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	if _, err := a.db.W.ExecContext(ctx, `UPDATE admins SET password_hash = ?, verified_at = COALESCE(verified_at, ?) WHERE id = ?`, h, now(), id); err != nil {
		return "", err
	}
	a.db.W.ExecContext(ctx, `DELETE FROM sessions WHERE admin_id = ?`, id)
	var email string
	a.db.W.QueryRowContext(ctx, `SELECT email FROM admins WHERE id = ?`, id).Scan(&email)
	return email, nil
}

// DeleteAccount removes the account and, by cascade, its projects and signups.
// Accounts without a password (Google or GitHub only) confirm with their email.
func (a *App) DeleteAccount(ctx context.Context, id int64, password string) error {
	var h, addr string
	if err := a.db.W.QueryRowContext(ctx, `SELECT password_hash, email FROM admins WHERE id = ?`, id).Scan(&h, &addr); err != nil {
		return err
	}
	if h == "" && !strings.EqualFold(strings.TrimSpace(password), addr) {
		return errors.New("Type your email address to confirm.")
	} else if h != "" && !checkPassword(h, password) {
		return errors.New("Wrong password.")
	}
	_, err := a.db.W.ExecContext(ctx, `DELETE FROM admins WHERE id = ?`, id)
	return err
}

func (a *App) lobbyFrom() string {
	return (&mail.Address{Name: "Lobby", Address: a.senderEmail()}).String()
}

func (a *App) mailLimitReached(ctx context.Context, ac *Account, plan Plan) {
	text, htm := email{
		Project:   "Lobby",
		Preheader: "New signups are being turned away.",
		Heading:   "Your waitlist is full",
		Paras: []string{
			fmt.Sprintf("Your waitlists have reached the %d signups included in the %s plan, so new visitors can't join right now.", plan.Signups, plan.Name),
			"Everyone already on the list keeps their place. Upgrade to Launch for unlimited signups and the form starts accepting people again straight away.",
		},
		CTA:    "Upgrade",
		URL:    a.cfg.BaseURL + "/billing",
		Footer: "You get this at most once a week while the limit is reached.",
	}.render()
	if err := a.mailer.Send(ctx, &Message{From: a.lobbyFrom(), To: ac.Email, Subject: "Your Lobby waitlist is full", Text: text, HTML: htm}); err != nil {
		a.log.Error("limit email", "err", err)
	}
}
