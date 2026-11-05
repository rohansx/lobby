package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

// The product profile is the one description of a product a launch kit is
// written from. Every save is a new, immutable version; approving a campaign's
// plan pins the version it approved. Missing fields are listed, never filled in.

type ProductProfile struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	Audience         string `json:"audience"`
	Problem          string `json:"problem"`
	Category         string `json:"category"`
	Pricing          string `json:"pricing"`
	Stage            string `json:"stage"` // idea | prelaunch | beta | live
	Availability     string `json:"availability"`
	FounderName      string `json:"founder_name"`
	FounderEmail     string `json:"founder_email"`
	FounderLinks     string `json:"founder_links"` // one URL per line
	Tagline          string `json:"tagline"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
	DemoURL          string `json:"demo_url"`
}

var profileStages = map[string]bool{"idea": true, "prelaunch": true, "beta": true, "live": true}

// maxProfileVersions keeps a project's history bounded; saving is a deliberate act, not autosave.
const maxProfileVersions = 200

type profileField struct {
	name string
	v    *string
	max  int
	url  bool
}

func (p *ProductProfile) fields() []profileField {
	return []profileField{
		{"name", &p.Name, 120, false},
		{"url", &p.URL, 500, true},
		{"audience", &p.Audience, 500, false},
		{"problem", &p.Problem, 1000, false},
		{"category", &p.Category, 100, false},
		{"pricing", &p.Pricing, 300, false},
		{"stage", &p.Stage, 20, false},
		{"availability", &p.Availability, 300, false},
		{"founder_name", &p.FounderName, 120, false},
		{"founder_email", &p.FounderEmail, 254, false},
		{"founder_links", &p.FounderLinks, 1000, false},
		{"tagline", &p.Tagline, 60, false},
		{"short_description", &p.ShortDescription, 160, false},
		{"long_description", &p.LongDescription, 1000, false},
		{"demo_url", &p.DemoURL, 500, true},
	}
}

// clean trims every field and checks lengths, URLs, the stage and the email.
func (p *ProductProfile) clean() error {
	for _, f := range p.fields() {
		*f.v = strings.TrimSpace(*f.v)
		if utf8.RuneCountInString(*f.v) > f.max {
			return &ValidationError{fmt.Sprintf("%s can be up to %d characters.", fieldLabel(f.name), f.max)}
		}
		if f.url && *f.v != "" && !isHTTPURL(*f.v) {
			return &ValidationError{fieldLabel(f.name) + " must start with http:// or https://."}
		}
	}
	if p.Stage != "" && !profileStages[p.Stage] {
		return &ValidationError{"Stage must be idea, prelaunch, beta or live."}
	}
	if p.FounderEmail != "" {
		if addr, err := mail.ParseAddress(p.FounderEmail); err != nil || addr.Address != p.FounderEmail {
			return &ValidationError{"The founder email isn't a valid address."}
		}
	}
	var links []string
	for _, l := range strings.Split(p.FounderLinks, "\n") {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		if !isHTTPURL(l) {
			return &ValidationError{"Each founder link must start with http:// or https://."}
		}
		links = append(links, l)
	}
	p.FounderLinks = strings.Join(links, "\n")
	return nil
}

// Missing lists the fields that are still empty.
func (p ProductProfile) Missing() []string {
	out := []string{}
	for _, f := range p.fields() {
		if *f.v == "" {
			out = append(out, f.name)
		}
	}
	return out
}

func fieldLabel(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	if strings.HasSuffix(s, " url") || s == "url" {
		s = strings.TrimSuffix(s, "url") + "URL"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type ProfileVersion struct {
	ID         int64          `json:"id"`
	Profile    ProductProfile `json:"profile"`
	CreatedAt  int64          `json:"created_at"`
	ApprovedAt *int64         `json:"approved_at"`
}

func scanProfile(row scanner) (*ProfileVersion, error) {
	pv := &ProfileVersion{}
	var data string
	err := row.Scan(&pv.ID, &data, &pv.CreatedAt, &pv.ApprovedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal([]byte(data), &pv.Profile)
	}
	return pv, err
}

func latestProfile(ctx context.Context, q querier, projectID int64) (*ProfileVersion, error) {
	return scanProfile(q.QueryRowContext(ctx, `SELECT id, data, created_at, approved_at FROM profile_versions
		WHERE project_id = ? ORDER BY id DESC LIMIT 1`, projectID))
}

// approvedProfile is the newest version a founder approved, or ErrNotFound.
func approvedProfile(ctx context.Context, q querier, projectID int64) (*ProfileVersion, error) {
	return scanProfile(q.QueryRowContext(ctx, `SELECT id, data, created_at, approved_at FROM profile_versions
		WHERE project_id = ? AND approved_at IS NOT NULL ORDER BY id DESC LIMIT 1`, projectID))
}

// SaveProfile stores a new version of a project's profile.
func (a *App) SaveProfile(ctx context.Context, projectID, by int64, p ProductProfile) (*ProfileVersion, error) {
	if err := p.clean(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(p)
	res, err := a.db.W.ExecContext(ctx, `INSERT INTO profile_versions (project_id, data, created_by, created_at)
		SELECT ?, ?, NULLIF(?, 0), ? WHERE (SELECT COUNT(*) FROM profile_versions WHERE project_id = ?) < ?`,
		projectID, string(b), by, now(), projectID, maxProfileVersions)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, &ValidationError{"This profile has been saved too many times. Contact support."}
	}
	id, _ := res.LastInsertId()
	return scanProfile(a.db.W.QueryRowContext(ctx, `SELECT id, data, created_at, approved_at FROM profile_versions WHERE id = ?`, id))
}

// profileView is the latest and approved versions plus what's still missing.
func (a *App) profileView(ctx context.Context, projectID int64) (map[string]any, error) {
	out := map[string]any{"latest": nil, "approved": nil, "missing": ProductProfile{}.Missing()}
	latest, err := latestProfile(ctx, a.db.R, projectID)
	if errors.Is(err, ErrNotFound) {
		return out, nil
	} else if err != nil {
		return nil, err
	}
	out["latest"], out["missing"] = latest, latest.Profile.Missing()
	if ap, err := approvedProfile(ctx, a.db.R, projectID); err == nil {
		out["approved"] = ap
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return out, nil
}
