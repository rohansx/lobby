package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Launch Pack campaigns (cloud only). A founder buys a pack, which opens a
// campaign in intake; operators review readiness, build a plan of up to
// maxCampaignTasks destinations, the founder approves it, the work is done and
// tracked per task, and a report closes it. Every status change goes through
// moveCampaign.

const (
	statusIntake           = "intake"
	statusReadiness        = "readiness"
	statusPlanning         = "planning"
	statusAwaitingApproval = "awaiting_approval"
	statusExecuting        = "executing"
	statusReporting        = "reporting"
	statusDone             = "done"
	statusCancelled        = "cancelled"
)

type actor string

const (
	byFounder  actor = "founder"
	byOperator actor = "operator"
)

// campaignMoves is every status change a campaign can make and who makes it.
// Any open campaign can also be cancelled by an operator.
var campaignMoves = map[[2]string]actor{
	{statusIntake, statusReadiness}:           byFounder,  // submit intake
	{statusReadiness, statusIntake}:           byOperator, // blockers: back to the founder
	{statusReadiness, statusPlanning}:         byOperator,
	{statusPlanning, statusReadiness}:         byOperator,
	{statusPlanning, statusAwaitingApproval}:  byOperator, // send for approval
	{statusAwaitingApproval, statusExecuting}: byFounder,  // approve
	{statusAwaitingApproval, statusPlanning}:  byFounder,  // request changes
	{statusExecuting, statusReporting}:        byOperator,
	{statusReporting, statusDone}:             byOperator, // publish report
}

// ErrConflict is a request that doesn't fit the campaign's current state (HTTP 409).
type ErrConflict struct{ Msg string }

func (e *ErrConflict) Error() string { return e.Msg }

func statusLabel(s string) string { return strings.ReplaceAll(s, "_", " ") }

func checkMove(from, to string, by actor) error {
	if to == statusCancelled && by == byOperator && from != statusDone && from != statusCancelled {
		return nil
	}
	if who, ok := campaignMoves[[2]string{from, to}]; ok && who == by {
		return nil
	}
	return &ErrConflict{fmt.Sprintf("This campaign can't go from %s to %s.", statusLabel(from), statusLabel(to))}
}

type ReadinessItem struct {
	Label string `json:"label"`
	OK    bool   `json:"ok"`
	Note  string `json:"note"`
}

type Campaign struct {
	ID                int64            `json:"id"`
	ProjectID         *int64           `json:"project_id"`
	ProjectSlug       string           `json:"project_slug"`
	OwnerID           int64            `json:"owner_id"`
	OwnerEmail        string           `json:"owner_email"`
	Status            string           `json:"status"`
	PriceCents        int64            `json:"price_cents"`
	PolarOrderID      string           `json:"polar_order_id,omitempty"`
	PaidAt            *int64           `json:"paid_at"`
	Readiness         []ReadinessItem  `json:"readiness"`
	ProfileVersionID  *int64           `json:"profile_version_id"`
	ApprovedAt        *int64           `json:"approved_at"`
	ChangesNote       string           `json:"changes_note"`
	ReportNote        string           `json:"report_note"`
	ReportPublishedAt *int64           `json:"report_published_at"`
	OperatorMinutes   int64            `json:"operator_minutes,omitempty"`
	Tasks             int64            `json:"tasks"`
	TaskStates        map[string]int64 `json:"task_states,omitempty"`  // operators only
	TaskMinutes       int64            `json:"task_minutes,omitempty"` // operators only
	CreatedAt         int64            `json:"created_at"`
	UpdatedAt         int64            `json:"updated_at"`
}

const campaignCols = `c.id, c.project_id, COALESCE(p.slug, ''), c.owner_id, a.email, c.status, c.price_cents,
	COALESCE(c.polar_order_id, ''), c.paid_at, c.readiness, c.profile_version_id, c.approved_at, c.changes_note,
	c.report_note, c.report_published_at, c.operator_minutes,
	(SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = c.id), c.created_at, c.updated_at
	FROM campaigns c JOIN admins a ON a.id = c.owner_id LEFT JOIN projects p ON p.id = c.project_id`

func scanCampaign(row scanner) (*Campaign, error) {
	c := &Campaign{}
	var readiness string
	err := row.Scan(&c.ID, &c.ProjectID, &c.ProjectSlug, &c.OwnerID, &c.OwnerEmail, &c.Status, &c.PriceCents,
		&c.PolarOrderID, &c.PaidAt, &readiness, &c.ProfileVersionID, &c.ApprovedAt, &c.ChangesNote,
		&c.ReportNote, &c.ReportPublishedAt, &c.OperatorMinutes, &c.Tasks, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal([]byte(readiness), &c.Readiness)
	}
	return c, err
}

// forFounder drops what only operators see: time spent, and the report until it's published.
func (c *Campaign) forFounder() *Campaign {
	out := *c
	out.OperatorMinutes = 0
	if out.ReportPublishedAt == nil {
		out.ReportNote = ""
	}
	return &out
}

// Campaign loads one campaign. ownerID 0 means any account (operators);
// otherwise another account's campaign is ErrNotFound.
func (a *App) Campaign(ctx context.Context, id, ownerID int64) (*Campaign, error) {
	return scanCampaign(a.db.W.QueryRowContext(ctx, `SELECT `+campaignCols+` WHERE c.id = ? AND (? = 0 OR c.owner_id = ?)`, id, ownerID, ownerID))
}

// Campaigns lists an account's campaigns, newest first, or with ownerID 0
// every campaign, the ones waiting on an operator first.
func (a *App) Campaigns(ctx context.Context, ownerID int64) ([]*Campaign, error) {
	order := `c.id DESC`
	if ownerID == 0 {
		order = `CASE c.status WHEN 'readiness' THEN 0 WHEN 'planning' THEN 1 WHEN 'executing' THEN 2 WHEN 'reporting' THEN 3
			WHEN 'intake' THEN 4 WHEN 'awaiting_approval' THEN 5 ELSE 6 END, c.updated_at`
	}
	rows, err := a.db.R.QueryContext(ctx, `SELECT `+campaignCols+` WHERE ? = 0 OR c.owner_id = ? ORDER BY `+order, ownerID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCampaign opens a campaign in intake for an account and puts the
// account on the pack plan. A repeated polarOrderID returns the campaign it
// already made, without emailing again; an empty one (a manual campaign) never matches.
func (a *App) CreateCampaign(ctx context.Context, ownerID, priceCents int64, polarOrderID string) (*Campaign, error) {
	polarOrderID = strings.TrimSpace(polarOrderID)
	if priceCents < 0 || priceCents > maxCents {
		return nil, &ValidationError{"The price must be between 0 and 100,000 dollars."}
	}
	if len(polarOrderID) > 200 {
		return nil, &ValidationError{"That order id is too long."}
	}
	if _, err := a.Account(ctx, ownerID); err != nil {
		return nil, err
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int64
	if polarOrderID != "" {
		err := tx.QueryRowContext(ctx, `SELECT id FROM campaigns WHERE polar_order_id = ?`, polarOrderID).Scan(&id)
		if err == nil {
			tx.Rollback()
			return a.Campaign(ctx, id, 0)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	t := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO campaigns (owner_id, status, price_cents, polar_order_id, paid_at, created_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?)`, ownerID, statusIntake, priceCents, polarOrderID, t, t, t)
	if err != nil {
		return nil, err
	}
	id, _ = res.LastInsertId()
	// A grandfathered subscription keeps its own plan until it ends; see basePlan.
	if _, err := tx.ExecContext(ctx, `UPDATE admins SET plan = 'pack' WHERE id = ?
		AND NOT (plan IN ('launch', 'pro') AND sub_status IN ('active', 'trialing', 'past_due'))`, ownerID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c, err := a.Campaign(ctx, id, 0)
	if err != nil {
		return nil, err
	}
	a.log.Info("campaign created", "campaign", c.ID, "account", ownerID, "price_cents", priceCents)
	a.mailCampaignStarted(ctx, c)
	return c, nil
}

// basePlan is the plan an account falls back to when a subscription ends:
// pack if it has ever bought a Launch Pack, free otherwise.
func (a *App) basePlan(ctx context.Context, ownerID int64) string {
	var n int
	a.db.W.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaigns WHERE owner_id = ?`, ownerID).Scan(&n)
	if n > 0 {
		return "pack"
	}
	return "free"
}

// moveCampaign is the one place a campaign changes status: it checks the
// move is allowed for this actor, checks the move's preconditions, applies
// its side effects in the same transaction and sends the emails that go with it.
func (a *App) moveCampaign(ctx context.Context, c *Campaign, to string, by actor, note string) (*Campaign, error) {
	if err := checkMove(c.Status, to, by); err != nil {
		return nil, err
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 2000 {
		return nil, &ValidationError{"Keep the note under 2,000 characters."}
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.ExecContext(ctx, `UPDATE campaigns SET status = ?, updated_at = ? WHERE id = ? AND status = ?`, to, t, c.ID, c.Status)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, &ErrConflict{"This campaign changed in the meantime. Reload and try again."}
	}
	founderAction := false
	switch to {
	case statusReadiness:
		if c.Status == statusIntake {
			if c.ProjectID == nil {
				return nil, &ErrConflict{"Choose the waitlist for this product first."}
			}
			if _, err := latestProfile(ctx, tx, *c.ProjectID); err != nil {
				return nil, &ErrConflict{"Fill in the product profile first."}
			}
		}
	case statusAwaitingApproval:
		var n int
		tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = ?`, c.ID).Scan(&n)
		if n == 0 {
			return nil, &ErrConflict{"Add at least one destination to the plan first."}
		}
		_, err = tx.ExecContext(ctx, `UPDATE campaigns SET changes_note = '' WHERE id = ?`, c.ID)
	case statusExecuting:
		if c.ProjectID == nil {
			return nil, &ErrConflict{"This campaign has no waitlist any more."}
		}
		pv, perr := latestProfile(ctx, tx, *c.ProjectID)
		if perr != nil {
			return nil, &ErrConflict{"There's no product profile to approve."}
		}
		// Approval pins the profile version the launch kit is written from.
		if _, err = tx.ExecContext(ctx, `UPDATE campaigns SET profile_version_id = ?, approved_at = ? WHERE id = ?`, pv.ID, t, c.ID); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE profile_versions SET approved_at = COALESCE(approved_at, ?) WHERE id = ?`, t, pv.ID); err != nil {
			return nil, err
		}
		res, err = tx.ExecContext(ctx, `UPDATE campaign_tasks SET state = CASE owner WHEN 'founder' THEN 'awaiting_founder' ELSE 'ready' END,
			updated_at = ? WHERE campaign_id = ? AND state = 'planned'`, t, c.ID)
		if err == nil {
			var n int
			tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = ? AND state = 'awaiting_founder'`, c.ID).Scan(&n)
			founderAction = n > 0
		}
	case statusPlanning:
		if c.Status == statusAwaitingApproval {
			_, err = tx.ExecContext(ctx, `UPDATE campaigns SET changes_note = ? WHERE id = ?`, note, c.ID)
		}
	case statusDone:
		if strings.TrimSpace(c.ReportNote) == "" {
			return nil, &ErrConflict{"Write the report before publishing it."}
		}
		_, err = tx.ExecContext(ctx, `UPDATE campaigns SET report_published_at = ? WHERE id = ?`, t, c.ID)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	a.log.Info("campaign moved", "campaign", c.ID, "from", c.Status, "to", to, "by", string(by))
	moved, err := a.Campaign(ctx, c.ID, 0)
	if err != nil {
		return nil, err
	}
	a.mailCampaignMove(ctx, c.Status, moved, note, founderAction)
	return moved, nil
}

// SetCampaignProject chooses the waitlist a campaign is for. Only during intake.
func (a *App) SetCampaignProject(ctx context.Context, c *Campaign, p *Project) error {
	if p.OwnerID != c.OwnerID {
		return ErrNotFound
	}
	res, err := a.db.W.ExecContext(ctx, `UPDATE campaigns SET project_id = ?, updated_at = ? WHERE id = ? AND status = ?`, p.ID, now(), c.ID, statusIntake)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &ErrConflict{"The waitlist can only be changed during intake."}
	}
	return nil
}

// campaignEdit is what an operator can set on a campaign besides its status.
type campaignEdit struct {
	Status          *string          `json:"status"`
	Readiness       *[]ReadinessItem `json:"readiness"`
	ReportNote      *string          `json:"report_note"`
	OperatorMinutes *int64           `json:"operator_minutes"`
}

const (
	maxReadinessItems = 30
	maxMinutes        = 100000
	maxCents          = 10000000
)

// EditCampaign applies an operator's changes; a status change goes through moveCampaign last.
func (a *App) EditCampaign(ctx context.Context, c *Campaign, e campaignEdit) (*Campaign, error) {
	readiness, note, minutes := c.Readiness, c.ReportNote, c.OperatorMinutes
	if e.Readiness != nil {
		if len(*e.Readiness) > maxReadinessItems {
			return nil, &ValidationError{fmt.Sprintf("Keep the checklist to %d items.", maxReadinessItems)}
		}
		readiness = []ReadinessItem{}
		for _, it := range *e.Readiness {
			it.Label, it.Note = strings.TrimSpace(it.Label), strings.TrimSpace(it.Note)
			if it.Label == "" || utf8.RuneCountInString(it.Label) > 120 || utf8.RuneCountInString(it.Note) > 500 {
				return nil, &ValidationError{"Each checklist item needs a label (up to 120 characters) and a note of up to 500."}
			}
			readiness = append(readiness, it)
		}
	}
	if e.ReportNote != nil {
		if note = strings.TrimSpace(*e.ReportNote); utf8.RuneCountInString(note) > 10000 {
			return nil, &ValidationError{"Keep the report under 10,000 characters."}
		}
	}
	if e.OperatorMinutes != nil {
		if minutes = *e.OperatorMinutes; minutes < 0 || minutes > maxMinutes {
			return nil, &ValidationError{"Minutes must be between 0 and 100,000."}
		}
	}
	if e.Status != nil && !checkStatus(*e.Status) {
		return nil, &ValidationError{"Unknown status."}
	}
	b, _ := json.Marshal(readiness)
	if _, err := a.db.W.ExecContext(ctx, `UPDATE campaigns SET readiness = ?, report_note = ?, operator_minutes = ?, updated_at = ? WHERE id = ?`,
		string(b), note, minutes, now(), c.ID); err != nil {
		return nil, err
	}
	c, err := a.Campaign(ctx, c.ID, 0)
	if err != nil || e.Status == nil || *e.Status == c.Status {
		return c, err
	}
	return a.moveCampaign(ctx, c, *e.Status, byOperator, "")
}

func checkStatus(s string) bool {
	switch s {
	case statusIntake, statusReadiness, statusPlanning, statusAwaitingApproval, statusExecuting, statusReporting, statusDone, statusCancelled:
		return true
	}
	return false
}
