package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// A campaign task is one destination on a plan: who does it, its state, and
// the evidence of what happened. Submitted and live are separate dates; a
// submitted form is never reported as a live listing.

// maxCampaignTasks is the most destinations a plan can hold. A maximum, not a quota.
const maxCampaignTasks = 20

var taskStates = map[string]bool{
	"planned": true, "needs_assets": true, "awaiting_approval": true, "awaiting_founder": true, "ready": true,
	"submitted": true, "live": true, "rejected": true, "blocked": true,
}

type Task struct {
	ID              int64  `json:"id"`
	CampaignID      int64  `json:"campaign_id"`
	DestinationID   int64  `json:"destination_id"`
	DestinationSlug string `json:"destination_slug"`
	DestinationName string `json:"destination_name"`
	DestinationURL  string `json:"destination_url"`
	Owner           string `json:"owner"` // staff | founder
	State           string `json:"state"`
	Reason          string `json:"reason"`
	FeeCents        int64  `json:"fee_cents"`
	Instructions    string `json:"instructions"`
	SubmittedAt     *int64 `json:"submitted_at"`
	LiveAt          *int64 `json:"live_at"`
	ListingURL      string `json:"listing_url"`
	Evidence        string `json:"evidence"`
	ExceptionNote   string `json:"exception_note"`
	Minutes         int64  `json:"minutes,omitempty"`
	CampaignLink    string `json:"campaign_link"` // hosted page tagged for this destination; empty without a waitlist
	projectSlug     string
	CreatedAt       int64 `json:"created_at"`
	UpdatedAt       int64 `json:"updated_at"`
}

const taskCols = `t.id, t.campaign_id, t.destination_id, d.slug, d.name, d.url, t.owner, t.state, t.reason, t.fee_cents,
	t.instructions, t.submitted_at, t.live_at, t.listing_url, t.evidence, t.exception_note, t.minutes, t.created_at, t.updated_at, COALESCE(p.slug, '')
	FROM campaign_tasks t JOIN destinations d ON d.id = t.destination_id
	JOIN campaigns c ON c.id = t.campaign_id LEFT JOIN projects p ON p.id = c.project_id`

func scanTask(row scanner) (*Task, error) {
	t := &Task{}
	err := row.Scan(&t.ID, &t.CampaignID, &t.DestinationID, &t.DestinationSlug, &t.DestinationName, &t.DestinationURL,
		&t.Owner, &t.State, &t.Reason, &t.FeeCents, &t.Instructions, &t.SubmittedAt, &t.LiveAt, &t.ListingURL,
		&t.Evidence, &t.ExceptionNote, &t.Minutes, &t.CreatedAt, &t.UpdatedAt, &t.projectSlug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (a *App) Tasks(ctx context.Context, campaignID int64) ([]*Task, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT `+taskCols+` WHERE t.campaign_id = ? ORDER BY t.id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		t.CampaignLink = a.CampaignLink(t.projectSlug, t.DestinationSlug, t.CampaignID)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (a *App) task(ctx context.Context, campaignID, id int64) (*Task, error) {
	t, err := scanTask(a.db.W.QueryRowContext(ctx, `SELECT `+taskCols+` WHERE t.id = ? AND t.campaign_id = ?`, id, campaignID))
	if err == nil {
		t.CampaignLink = a.CampaignLink(t.projectSlug, t.DestinationSlug, t.CampaignID)
	}
	return t, err
}

func (t *Task) clean() error {
	if t.Owner != "staff" && t.Owner != "founder" {
		return &ValidationError{"Owner must be staff or founder."}
	}
	if !taskStates[t.State] {
		return &ValidationError{"Unknown task state."}
	}
	for _, f := range []struct {
		label string
		v     *string
		max   int
	}{{"Reason", &t.Reason, 1000}, {"Instructions", &t.Instructions, 4000}, {"Listing URL", &t.ListingURL, 500},
		{"Evidence", &t.Evidence, 4000}, {"Exception note", &t.ExceptionNote, 2000}} {
		*f.v = strings.TrimSpace(*f.v)
		if utf8.RuneCountInString(*f.v) > f.max {
			return &ValidationError{fmt.Sprintf("%s can be up to %d characters.", f.label, f.max)}
		}
	}
	if t.ListingURL != "" && !isHTTPURL(t.ListingURL) {
		return &ValidationError{"The listing URL must start with http:// or https://."}
	}
	if t.FeeCents < 0 || t.FeeCents > maxCents {
		return &ValidationError{"The fee must be between 0 and 100,000 dollars."}
	}
	if t.Minutes < 0 || t.Minutes > maxMinutes {
		return &ValidationError{"Minutes must be between 0 and 100,000."}
	}
	return nil
}

// stamp fills the submitted or live date when a task reaches that state without one.
func (t *Task) stamp() {
	n := now()
	if t.State == "submitted" && t.SubmittedAt == nil {
		t.SubmittedAt = &n
	}
	if t.State == "live" && t.LiveAt == nil {
		t.LiveAt = &n
	}
}

// taskEdit is an operator's change to a task; nil fields stay as they are.
// A date of 0 clears it.
type taskEdit struct {
	DestinationID int64   `json:"destination_id"` // when adding
	Owner         *string `json:"owner"`
	State         *string `json:"state"`
	Reason        *string `json:"reason"`
	FeeCents      *int64  `json:"fee_cents"`
	Instructions  *string `json:"instructions"`
	SubmittedAt   *int64  `json:"submitted_at"`
	LiveAt        *int64  `json:"live_at"`
	ListingURL    *string `json:"listing_url"`
	Evidence      *string `json:"evidence"`
	ExceptionNote *string `json:"exception_note"`
	Minutes       *int64  `json:"minutes"`
}

func (e taskEdit) apply(t *Task) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&t.Owner, e.Owner)
	set(&t.State, e.State)
	set(&t.Reason, e.Reason)
	set(&t.Instructions, e.Instructions)
	set(&t.ListingURL, e.ListingURL)
	set(&t.Evidence, e.Evidence)
	set(&t.ExceptionNote, e.ExceptionNote)
	if e.FeeCents != nil {
		t.FeeCents = *e.FeeCents
	}
	if e.Minutes != nil {
		t.Minutes = *e.Minutes
	}
	for _, d := range []struct {
		dst **int64
		v   *int64
	}{{&t.SubmittedAt, e.SubmittedAt}, {&t.LiveAt, e.LiveAt}} {
		if d.v != nil {
			if *d.v == 0 {
				*d.dst = nil
			} else {
				v := *d.v
				*d.dst = &v
			}
		}
	}
}

// AddTask puts a destination on a campaign's plan, while it's being planned.
func (a *App) AddTask(ctx context.Context, c *Campaign, e taskEdit) (*Task, error) {
	d, err := a.DestinationByID(ctx, e.DestinationID)
	if err != nil || !d.Active {
		return nil, &ValidationError{"Choose an active destination."}
	}
	t := &Task{Owner: "staff", State: "planned", FeeCents: d.FeeCents}
	if d.Mode == "founder" {
		t.Owner = "founder"
	}
	e.apply(t)
	if err := t.clean(); err != nil {
		return nil, err
	}
	tx, err := a.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var status string
	var n int
	tx.QueryRowContext(ctx, `SELECT status FROM campaigns WHERE id = ?`, c.ID).Scan(&status)
	if status != statusPlanning {
		return nil, &ErrConflict{"Destinations can only be added while the plan is being made."}
	}
	tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_tasks WHERE campaign_id = ?`, c.ID).Scan(&n)
	if n >= maxCampaignTasks {
		return nil, &ErrConflict{fmt.Sprintf("A plan can have up to %d destinations.", maxCampaignTasks)}
	}
	ts := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO campaign_tasks (campaign_id, destination_id, owner, state, reason, fee_cents, instructions,
		submitted_at, live_at, listing_url, evidence, exception_note, minutes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, d.ID, t.Owner, t.State, t.Reason, t.FeeCents, t.Instructions, t.SubmittedAt, t.LiveAt, t.ListingURL, t.Evidence,
		t.ExceptionNote, t.Minutes, ts, ts)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, &ErrConflict{"That destination is already on the plan."}
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a.task(ctx, c.ID, id)
}

func (a *App) saveTask(ctx context.Context, t *Task) (*Task, error) {
	t.stamp()
	if _, err := a.db.W.ExecContext(ctx, `UPDATE campaign_tasks SET owner = ?, state = ?, reason = ?, fee_cents = ?, instructions = ?,
		submitted_at = ?, live_at = ?, listing_url = ?, evidence = ?, exception_note = ?, minutes = ?, updated_at = ? WHERE id = ?`,
		t.Owner, t.State, t.Reason, t.FeeCents, t.Instructions, t.SubmittedAt, t.LiveAt, t.ListingURL, t.Evidence,
		t.ExceptionNote, t.Minutes, now(), t.ID); err != nil {
		return nil, err
	}
	return a.task(ctx, t.CampaignID, t.ID)
}

// EditTask applies an operator's change. Moving a task to awaiting_founder emails the founder.
func (a *App) EditTask(ctx context.Context, c *Campaign, id int64, e taskEdit) (*Task, error) {
	if c.Status == statusCancelled {
		return nil, &ErrConflict{"This campaign is cancelled."}
	}
	t, err := a.task(ctx, c.ID, id)
	if err != nil {
		return nil, err
	}
	was := t.State
	e.apply(t)
	if err := t.clean(); err != nil {
		return nil, err
	}
	if t, err = a.saveTask(ctx, t); err != nil {
		return nil, err
	}
	if was != "awaiting_founder" && t.State == "awaiting_founder" && t.Owner == "founder" {
		a.mailFounderAction(ctx, c)
	}
	return t, nil
}

// founderTaskEdit is all a founder can change, on their own tasks only.
type founderTaskEdit struct {
	State      string  `json:"state"` // submitted | live
	ListingURL *string `json:"listing_url"`
	Evidence   *string `json:"evidence"`
}

// FounderEditTask records a founder doing their part of an approved plan.
func (a *App) FounderEditTask(ctx context.Context, c *Campaign, id int64, e founderTaskEdit) (*Task, error) {
	t, err := a.task(ctx, c.ID, id)
	if err != nil {
		return nil, err
	}
	if t.Owner != "founder" {
		return nil, &ErrConflict{"The Lobby team does this one; it updates as they go."}
	}
	if c.Status != statusExecuting {
		return nil, &ErrConflict{"Tasks can be updated once the plan is approved, until the report is written."}
	}
	if e.State != "submitted" && e.State != "live" {
		return nil, &ValidationError{"State must be submitted or live."}
	}
	t.State = e.State
	if e.ListingURL != nil {
		t.ListingURL = *e.ListingURL
	}
	if e.Evidence != nil {
		t.Evidence = *e.Evidence
	}
	if err := t.clean(); err != nil {
		return nil, err
	}
	return a.saveTask(ctx, t)
}

// DeleteTask takes a destination off the plan while it's being planned.
func (a *App) DeleteTask(ctx context.Context, c *Campaign, id int64) error {
	if c.Status != statusPlanning {
		return &ErrConflict{"Destinations can only be removed while the plan is being made."}
	}
	res, err := a.db.W.ExecContext(ctx, `DELETE FROM campaign_tasks WHERE id = ? AND campaign_id = ?`, id, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// taskStats counts every campaign's tasks by state and sums their minutes, for the operator queue.
func (a *App) taskStats(ctx context.Context) (map[int64]map[string]int64, map[int64]int64, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT campaign_id, state, COUNT(*), SUM(minutes) FROM campaign_tasks GROUP BY campaign_id, state`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	states, minutes := map[int64]map[string]int64{}, map[int64]int64{}
	for rows.Next() {
		var id, n, m int64
		var state string
		if err := rows.Scan(&id, &state, &n, &m); err != nil {
			return nil, nil, err
		}
		if states[id] == nil {
			states[id] = map[string]int64{}
		}
		states[id][state] = n
		minutes[id] += m
	}
	return states, minutes, rows.Err()
}
