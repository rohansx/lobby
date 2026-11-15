package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A claim hands over an account the founder built in their own product (for
// a sales prospect, say) to a real person. Lobby's part is the link, a
// verified email and a one-time code; the product redeems the code for its
// own account id and swaps in the email. Lobby never sees product credentials.
//
//	create → /c/{token} → email entered → /c/{token}/confirm/{sig}
//	       → redirect to the invite URL with ?claim={code} → POST /v1/claims/redeem

const (
	claimSource         = "claim" // shows as the signup's source
	claimDefaultTTL     = 30 * 86400
	claimMaxTTL         = 365 * 86400
	claimCodeTTL        = 15 * 60
	claimMaxRef         = 200
	claimMaxLabel       = 120
	claimSendsPerDay    = 5
	claimLinkPurpose    = "claim"
	claimConfirmPurpose = "claim-confirm"
)

var ErrInvalidClaim = errors.New("this claim is invalid, expired, revoked or already used")

type Claim struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"-"`
	AccountRef  string `json:"account_ref"`
	Label       string `json:"label"`
	EmailLock   string `json:"email_lock"`    // as the founder typed it; empty means anyone with the link
	Pending     string `json:"pending_email"` // typed on the claim page, not confirmed yet
	Email       string `json:"email"`         // the address confirmed from the email link
	SignupID    *int64 `json:"signup_id"`
	CreatedAt   int64  `json:"created_at"`
	ExpiresAt   int64  `json:"expires_at"`
	OpenedAt    *int64 `json:"opened_at"`
	VerifiedAt  *int64 `json:"verified_at"`
	ClaimedAt   *int64 `json:"claimed_at"`
	RevokedAt   *int64 `json:"revoked_at"`
	codeExpires *int64
}

// State is what the dashboard shows: open, opened, verified, claimed, revoked or expired.
func (c *Claim) State() string {
	switch {
	case c.ClaimedAt != nil:
		return "claimed"
	case c.RevokedAt != nil:
		return "revoked"
	case c.ExpiresAt <= now():
		return "expired"
	case c.VerifiedAt != nil:
		return "verified"
	case c.OpenedAt != nil:
		return "opened"
	}
	return "open"
}

func (c *Claim) live() bool {
	s := c.State()
	return s != "claimed" && s != "revoked" && s != "expired"
}

func (c *Claim) MarshalJSON() ([]byte, error) {
	type plain Claim
	return json.Marshal(struct {
		*plain
		State string `json:"state"`
	}{(*plain)(c), c.State()})
}

const claimCols = `id, project_id, account_ref, label, email_lock, pending_email, verified_email, signup_id, created_at, expires_at,
	opened_at, verified_at, claimed_at, revoked_at, code_expires_at`

func scanClaim(row scanner) (*Claim, error) {
	c := &Claim{}
	err := row.Scan(&c.ID, &c.ProjectID, &c.AccountRef, &c.Label, &c.EmailLock, &c.Pending, &c.Email, &c.SignupID, &c.CreatedAt,
		&c.ExpiresAt, &c.OpenedAt, &c.VerifiedAt, &c.ClaimedAt, &c.RevokedAt, &c.codeExpires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

type ClaimInput struct {
	AccountRef string `json:"account_ref"`
	Email      string `json:"email"`
	Label      string `json:"label"`
	ExpiresIn  int64  `json:"expires_in"` // seconds; 0 is the 30-day default
}

// claimTarget is where a verified prospect is sent: the invite URL, else the site.
func claimTarget(p *Project) string {
	if p.InviteURL != "" {
		return p.InviteURL
	}
	return p.SiteURL
}

// CreateClaim opens a claim for one account. An open claim for the same
// account is revoked, so only the newest link works.
func (a *App) CreateClaim(ctx context.Context, p *Project, in ClaimInput) (*Claim, error) {
	in.AccountRef, in.Label, in.Email = strings.TrimSpace(in.AccountRef), strings.TrimSpace(in.Label), strings.TrimSpace(in.Email)
	switch {
	case claimTarget(p) == "":
		return nil, &ValidationError{"Set the waitlist's Invite URL (or Site URL) first: it's where people land after claiming."}
	case in.AccountRef == "" || len(in.AccountRef) > claimMaxRef:
		return nil, &ValidationError{fmt.Sprintf("account_ref is required, up to %d characters.", claimMaxRef)}
	case utf8.RuneCountInString(in.Label) > claimMaxLabel:
		return nil, &ValidationError{fmt.Sprintf("Keep the label under %d characters.", claimMaxLabel)}
	case in.ExpiresIn < 0 || in.ExpiresIn > claimMaxTTL:
		return nil, &ValidationError{"expires_in is seconds, up to a year."}
	}
	if in.Email != "" {
		email, _, err := parseEmail(in.Email)
		if err != nil {
			return nil, &ValidationError{err.Error()}
		}
		in.Email = email
	}
	ttl := in.ExpiresIn
	if ttl == 0 {
		ttl = claimDefaultTTL
	}
	if a.ownerSuspended(ctx, p.OwnerID) {
		return nil, ErrWaitlistFull
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t := now()
	if _, err := tx.ExecContext(ctx, `UPDATE claims SET revoked_at = ? WHERE project_id = ? AND account_ref = ?
		AND claimed_at IS NULL AND revoked_at IS NULL`, t, p.ID, in.AccountRef); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO claims (project_id, account_ref, label, email_lock, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, p.ID, in.AccountRef, in.Label, in.Email, t, t+ttl)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a.ClaimByID(ctx, p, id)
}

func (a *App) ClaimByID(ctx context.Context, p *Project, id int64) (*Claim, error) {
	return scanClaim(a.db.W.QueryRowContext(ctx, `SELECT `+claimCols+` FROM claims WHERE id = ? AND project_id = ?`, id, p.ID))
}

// ClaimByToken resolves a claim link. The project comes from the claim.
func (a *App) ClaimByToken(ctx context.Context, tok string) (*Project, *Claim, error) {
	id, ok := a.parseToken(claimLinkPurpose, tok)
	if !ok {
		return nil, nil, ErrNotFound
	}
	c, err := scanClaim(a.db.W.QueryRowContext(ctx, `SELECT `+claimCols+` FROM claims WHERE id = ?`, id))
	if err != nil {
		return nil, nil, err
	}
	p, err := a.ProjectByID(ctx, c.ProjectID)
	return p, c, err
}

func (a *App) ListClaims(ctx context.Context, p *Project) ([]*Claim, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT `+claimCols+` FROM claims WHERE project_id = ? ORDER BY id DESC LIMIT 500`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Claim{}
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *App) RevokeClaim(ctx context.Context, p *Project, id int64) error {
	res, err := a.db.W.ExecContext(ctx, `UPDATE claims SET revoked_at = ? WHERE id = ? AND project_id = ? AND claimed_at IS NULL AND revoked_at IS NULL`,
		now(), id, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidClaim
	}
	return nil
}

func (a *App) ClaimURL(c *Claim) string {
	id := strconv.FormatInt(c.ID, 36)
	return a.cfg.BaseURL + "/c/" + id + "." + a.tokenSig(claimLinkPurpose, id)
}

// MarkClaimOpened records the first visit, so the founder knows a prospect looked.
func (a *App) MarkClaimOpened(ctx context.Context, p *Project, c *Claim) {
	if c.OpenedAt != nil || !c.live() {
		return
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE claims SET opened_at = ? WHERE id = ? AND opened_at IS NULL`, now(), c.ID)
	if n, _ := res.RowsAffected(); err != nil || n == 0 {
		return
	}
	if err := a.emit(ctx, tx, p.ID, "claim.opened", map[string]any{"claim": c}); err != nil {
		a.log.Error("claim opened webhook", "err", err)
		return
	}
	if tx.Commit() == nil {
		a.kick()
	}
}

func (a *App) claimConfirmSig(id int64, email string) string {
	return a.tokenSig(claimConfirmPurpose, strconv.FormatInt(id, 36)+":"+normalizeEmail(email))
}

// StartClaim checks the address the prospect typed and emails them a
// confirmation link. It returns that link (tests use it; callers ignore it).
func (a *App) StartClaim(ctx context.Context, p *Project, c *Claim, raw string) (string, error) {
	c, err := a.ClaimByID(ctx, p, c.ID) // re-read: it may have been revoked since the page loaded
	if err != nil || !c.live() {
		return "", ErrInvalidClaim
	}
	addr, domain, err := parseEmail(raw)
	if err != nil {
		return "", &ValidationError{err.Error()}
	}
	if c.EmailLock != "" && normalizeEmail(addr) != normalizeEmail(c.EmailLock) {
		return "", &ValidationError{"This account is set aside for a different email address. Use the address it was sent to."}
	}
	if c.EmailLock != "" {
		addr = c.EmailLock // hand the product exactly the address the founder named
	}
	if !a.spam.Allow("claim:"+strconv.FormatInt(c.ID, 10), claimSendsPerDay, 86400e9) {
		return "", &ValidationError{"Too many emails for this link today. Try again tomorrow."}
	}
	if a.spam.IsDisposable(domain) {
		return "", &ValidationError{"Please use a permanent email address."}
	}
	if !a.spam.HasMX(ctx, domain) {
		return "", &ValidationError{"That email domain can't receive mail. Check for a typo?"}
	}
	if a.isSuppressed(ctx, p.ID, addr) {
		return "", &ValidationError{"We can't send email to that address. Try another one."}
	}
	// A new address voids any code issued for the previous one, so a code can
	// only ever hand over the address that was actually confirmed.
	if _, err := a.db.W.ExecContext(ctx, `UPDATE claims SET pending_email = ?, code_hash = '', code_expires_at = NULL WHERE id = ?`, addr, c.ID); err != nil {
		return "", err
	}
	link := strings.TrimPrefix(a.ClaimURL(c), a.cfg.BaseURL)
	link = a.cfg.BaseURL + link + "/confirm/" + a.claimConfirmSig(c.ID, addr)
	text, htm := email{
		Project:   p.Name,
		Preheader: "One click and your " + p.Name + " account is yours.",
		Heading:   "Claim your " + p.Name + " account",
		Paras:     []string{"Your " + p.Name + " account is ready. Confirm this is your address and we'll take you straight to it."},
		CTA:       "Claim my account",
		URL:       link,
		Footer:    "If you weren't expecting this, ignore it. Nothing happens until you click.",
	}.render()
	b, _ := json.Marshal(&Message{From: a.fromAddress(p), To: addr, ReplyTo: p.ReplyTo, Subject: "Claim your " + p.Name + " account", Text: text, HTML: htm})
	if _, err := a.db.W.ExecContext(ctx, `INSERT INTO jobs (kind, payload, run_at, created_at) VALUES ('email.lobby', ?, ?, ?)`, string(b), now(), now()); err != nil {
		return "", err
	}
	a.kick()
	return link, nil
}

// ConfirmClaim checks the confirmation link, records that address as
// verified and issues a fresh one-time code, returning where to send the
// browser. The page calls it on a button press, not on GET, so mail scanners
// that prefetch links can't spend it.
func (a *App) ConfirmClaim(ctx context.Context, p *Project, id int64, sig string) (string, error) {
	c, err := a.ClaimByID(ctx, p, id)
	if err != nil || !c.live() || c.Pending == "" || !hmacEqual(sig, a.claimConfirmSig(c.ID, c.Pending)) {
		return "", ErrInvalidClaim
	}
	code := "cl_" + randHex(16)
	t := now()
	res, err := a.db.W.ExecContext(ctx, `UPDATE claims SET code_hash = ?, code_expires_at = ?, verified_email = pending_email,
		verified_at = COALESCE(verified_at, ?) WHERE id = ? AND pending_email = ?`, hashKey(code), t+claimCodeTTL, t, c.ID, c.Pending)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrInvalidClaim // the address changed since this link was sent
	}
	u, err := url.Parse(claimTarget(p))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("claim", code)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// RedeemClaim is the product's call: it spends the code and returns the
// account and the verified email. The prospect joins the list as joined.
func (a *App) RedeemClaim(ctx context.Context, p *Project, code string) (*Claim, error) {
	plan, _ := a.planForOwner(ctx, p.OwnerID) // before the transaction: the writer pool has one connection
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	c, err := scanClaim(tx.QueryRowContext(ctx, `SELECT `+claimCols+` FROM claims WHERE project_id = ? AND code_hash = ? AND code_hash != ''`, p.ID, hashKey(strings.TrimSpace(code))))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidClaim
	} else if err != nil {
		return nil, err
	}
	if !c.live() || c.Email == "" || c.codeExpires == nil || *c.codeExpires <= now() {
		return nil, ErrInvalidClaim
	}
	signupID, err := a.claimSignup(ctx, tx, p, plan, c.Email)
	if err != nil {
		return nil, err
	}
	t := now()
	if _, err := tx.ExecContext(ctx, `UPDATE claims SET claimed_at = ?, signup_id = ?, code_hash = '' WHERE id = ?`, t, signupID, c.ID); err != nil {
		return nil, err
	}
	c.ClaimedAt, c.SignupID = &t, signupID
	if err := a.emit(ctx, tx, p.ID, "claim.claimed", map[string]any{"claim": c}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	a.kick()
	return c, nil
}

// claimSignup puts the prospect on the list as joined, like a tester: never
// subscribed to waitlist mail. Someone already on the list is marked joined.
// ponytail: past the plan's signup limit the claim still succeeds, just
// without a signup row; the product's handover matters more than the list.
func (a *App) claimSignup(ctx context.Context, tx *sql.Tx, p *Project, plan Plan, email string) (*int64, error) {
	norm := normalizeEmail(email)
	existing, err := scanSignup(tx.QueryRowContext(ctx, `SELECT `+signupCols+` FROM signups WHERE project_id = ? AND email_norm = ?`, p.ID, norm))
	switch {
	case err == nil && existing.Status == StatusRemoved:
		return nil, nil
	case err == nil:
		if _, err := a.markJoined(ctx, tx, p, existing.ID); err != nil {
			return nil, err
		}
		return &existing.ID, nil
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}
	if plan.Signups >= 0 && a.signupsUsed(ctx, tx, p.OwnerID) >= plan.Signups {
		return nil, nil
	}
	t := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO signups (project_id, email, email_norm, status, base_position, referral_code,
		utm_source, consent_version, created_at, confirmed_at, joined_at, unsubscribed_at)
		VALUES (?, ?, ?, ?, (SELECT COALESCE(MAX(base_position), 0) + 1 FROM signups WHERE project_id = ?), ?, ?, 0, ?, ?, ?, ?)`,
		p.ID, email, norm, StatusJoined, p.ID, randCode(8), claimSource, t, t, t, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &id, nil
}
