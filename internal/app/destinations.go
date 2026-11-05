package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Destinations are the operator's catalogue of launch directories and
// communities. Founders never see the catalogue, only the destinations on
// their own plan. Nothing is seeded: the operator researches and verifies each one.

type Destination struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Audience       string `json:"audience"`
	Stages         string `json:"stages"` // comma-separated profile stages accepted; empty is any
	FeeCents       int64  `json:"fee_cents"`
	FeeNote        string `json:"fee_note"`
	Fields         string `json:"fields"` // what the form asks for
	Rules          string `json:"rules"`  // the platform's rules, in the operator's words
	Mode           string `json:"mode"`   // staff | founder | guided
	Active         bool   `json:"active"`
	LastVerifiedAt *int64 `json:"last_verified_at"`
	Notes          string `json:"notes"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

const destinationCols = `id, slug, name, url, audience, stages, fee_cents, fee_note, fields, rules, mode, active,
	last_verified_at, notes, created_at, updated_at`

func scanDestination(row scanner) (*Destination, error) {
	d := &Destination{}
	err := row.Scan(&d.ID, &d.Slug, &d.Name, &d.URL, &d.Audience, &d.Stages, &d.FeeCents, &d.FeeNote, &d.Fields,
		&d.Rules, &d.Mode, &d.Active, &d.LastVerifiedAt, &d.Notes, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

func (d *Destination) clean() error {
	d.Slug = strings.ToLower(strings.TrimSpace(d.Slug))
	if !slugRe.MatchString(d.Slug) {
		return &ValidationError{"Slug must be lowercase letters, digits and dashes."}
	}
	d.Name, d.URL = strings.TrimSpace(d.Name), strings.TrimSpace(d.URL)
	if d.Name == "" {
		return &ValidationError{"Name is required."}
	}
	if !isHTTPURL(d.URL) {
		return &ValidationError{"URL must start with http:// or https://."}
	}
	for _, f := range []struct {
		label string
		v     *string
		max   int
	}{{"Name", &d.Name, 120}, {"URL", &d.URL, 500}, {"Audience", &d.Audience, 500}, {"Fee note", &d.FeeNote, 300},
		{"Fields", &d.Fields, 4000}, {"Rules", &d.Rules, 4000}, {"Notes", &d.Notes, 4000}} {
		*f.v = strings.TrimSpace(*f.v)
		if utf8.RuneCountInString(*f.v) > f.max {
			return &ValidationError{fmt.Sprintf("%s can be up to %d characters.", f.label, f.max)}
		}
	}
	var stages []string
	for _, s := range strings.Split(d.Stages, ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if !profileStages[s] {
			return &ValidationError{"Stages must be idea, prelaunch, beta or live."}
		}
		stages = append(stages, s)
	}
	d.Stages = strings.Join(stages, ",")
	if d.FeeCents < 0 || d.FeeCents > maxCents {
		return &ValidationError{"The fee must be between 0 and 100,000 dollars."}
	}
	if d.Mode != "staff" && d.Mode != "founder" && d.Mode != "guided" {
		return &ValidationError{"Mode must be staff, founder or guided."}
	}
	return nil
}

// Destinations lists the catalogue. A stage keeps the active ones that accept it.
func (a *App) Destinations(ctx context.Context, stage string) ([]*Destination, error) {
	q, args := `SELECT `+destinationCols+` FROM destinations ORDER BY name`, []any{}
	if stage != "" {
		q = `SELECT ` + destinationCols + ` FROM destinations WHERE active = 1
			AND (stages = '' OR ',' || stages || ',' LIKE '%,' || ? || ',%') ORDER BY name`
		args = append(args, stage)
	}
	rows, err := a.db.R.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Destination{}
	for rows.Next() {
		d, err := scanDestination(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (a *App) DestinationByID(ctx context.Context, id int64) (*Destination, error) {
	return scanDestination(a.db.W.QueryRowContext(ctx, `SELECT `+destinationCols+` FROM destinations WHERE id = ?`, id))
}

// SaveDestination inserts d when its ID is 0 and updates it otherwise.
func (a *App) SaveDestination(ctx context.Context, d *Destination) (*Destination, error) {
	if err := d.clean(); err != nil {
		return nil, err
	}
	t := now()
	var err error
	if d.ID == 0 {
		var res sql.Result
		res, err = a.db.W.ExecContext(ctx, `INSERT INTO destinations (slug, name, url, audience, stages, fee_cents, fee_note, fields, rules,
			mode, active, last_verified_at, notes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.Slug, d.Name, d.URL, d.Audience, d.Stages, d.FeeCents, d.FeeNote, d.Fields, d.Rules, d.Mode, d.Active, d.LastVerifiedAt, d.Notes, t, t)
		if err == nil {
			d.ID, _ = res.LastInsertId()
		}
	} else {
		_, err = a.db.W.ExecContext(ctx, `UPDATE destinations SET slug = ?, name = ?, url = ?, audience = ?, stages = ?, fee_cents = ?,
			fee_note = ?, fields = ?, rules = ?, mode = ?, active = ?, last_verified_at = ?, notes = ?, updated_at = ? WHERE id = ?`,
			d.Slug, d.Name, d.URL, d.Audience, d.Stages, d.FeeCents, d.FeeNote, d.Fields, d.Rules, d.Mode, d.Active, d.LastVerifiedAt, d.Notes, t, d.ID)
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, &ValidationError{"There's already a destination with that slug."}
		}
		return nil, err
	}
	return a.DestinationByID(ctx, d.ID)
}

// DeleteDestination refuses while a campaign plan uses it; deactivate it instead.
func (a *App) DeleteDestination(ctx context.Context, id int64) error {
	res, err := a.db.W.ExecContext(ctx, `DELETE FROM destinations WHERE id = ?`, id)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return &ErrConflict{"A campaign plan uses this destination. Mark it inactive instead."}
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
