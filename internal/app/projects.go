package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var ErrNotFound = errors.New("not found")

type Project struct {
	ID                int64  `json:"id"`
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	PublicKey         string `json:"public_key"`
	SecretKeyHash     string `json:"-"`
	AllowedOrigins    string `json:"allowed_origins"`
	DOIEnabled        bool   `json:"doi_enabled"`
	PositionMode      string `json:"position_mode"`
	PointsPerReferral int    `json:"points_per_referral"`
	ReferralIPCap     int    `json:"referral_ip_cap"`
	ReferralHourlyCap int    `json:"referral_hourly_cap"`
	ShowPosition      bool   `json:"show_position"`
	CounterFloor      int    `json:"counter_floor"`
	FromName          string `json:"from_name"`
	ReplyTo           string `json:"reply_to"`
	Headline          string `json:"headline"`
	Description       string `json:"description"`
	SiteURL           string `json:"site_url"`
	RedirectURL       string `json:"redirect_url"`
	InviteURL         string `json:"invite_url"`
	ConsentText       string `json:"consent_text"`
	ConsentVersion    int    `json:"consent_version"`
	CreatedAt         int64  `json:"created_at"`
}

const projectCols = `id, slug, name, public_key, secret_key_hash, allowed_origins, doi_enabled, position_mode,
	points_per_referral, referral_ip_cap, referral_hourly_cap, show_position, counter_floor, from_name, reply_to,
	headline, description, site_url, redirect_url, invite_url, consent_text, consent_version, created_at`

type scanner interface{ Scan(...any) error }

func scanProject(row scanner) (*Project, error) {
	p := &Project{}
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.PublicKey, &p.SecretKeyHash, &p.AllowedOrigins, &p.DOIEnabled,
		&p.PositionMode, &p.PointsPerReferral, &p.ReferralIPCap, &p.ReferralHourlyCap, &p.ShowPosition,
		&p.CounterFloor, &p.FromName, &p.ReplyTo, &p.Headline, &p.Description, &p.SiteURL, &p.RedirectURL, &p.InviteURL,
		&p.ConsentText, &p.ConsentVersion, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// CreateProject returns the project and its secret key, which is only ever shown once.
func (a *App) CreateProject(ctx context.Context, name, slug string) (*Project, string, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", errors.New("name is required")
	}
	if !slugRe.MatchString(slug) {
		return nil, "", errors.New("slug must be lowercase letters, digits and dashes")
	}
	pk := "pk_" + randHex(12)
	sk := "sk_" + randHex(24)
	res, err := a.db.W.ExecContext(ctx, `INSERT INTO projects (slug, name, public_key, secret_key_hash, from_name, headline, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, slug, name, pk, hashKey(sk), name, "Join the "+name+" waitlist", now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, "", errors.New("slug is already taken")
		}
		return nil, "", err
	}
	id, _ := res.LastInsertId()
	p, err := a.ProjectByID(ctx, id)
	return p, sk, err
}

func (a *App) RotateSecret(ctx context.Context, id int64) (string, error) {
	sk := "sk_" + randHex(24)
	_, err := a.db.W.ExecContext(ctx, `UPDATE projects SET secret_key_hash = ? WHERE id = ?`, hashKey(sk), id)
	return sk, err
}

func (a *App) ProjectByID(ctx context.Context, id int64) (*Project, error) {
	return scanProject(a.db.W.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE id = ?`, id))
}

func (a *App) ProjectByPublicKey(ctx context.Context, pk string) (*Project, error) {
	return scanProject(a.db.W.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE public_key = ?`, pk))
}

func (a *App) ProjectBySlug(ctx context.Context, slug string) (*Project, error) {
	return scanProject(a.db.W.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE slug = ?`, slug))
}

func (a *App) ProjectBySecret(ctx context.Context, sk string) (*Project, error) {
	return scanProject(a.db.W.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE secret_key_hash = ?`, hashKey(sk)))
}

func (a *App) Projects(ctx context.Context) ([]*Project, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT `+projectCols+` FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (a *App) UpdateProject(ctx context.Context, p *Project) error {
	if p.PositionMode != "fifo" && p.PositionMode != "points" {
		return errors.New("position mode must be fifo or points")
	}
	for _, u := range []string{p.SiteURL, p.RedirectURL, p.InviteURL} {
		if u != "" && !isHTTPURL(u) {
			return errors.New("URLs must start with http:// or https://")
		}
	}
	var origins []string
	for _, o := range strings.FieldsFunc(p.AllowedOrigins, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == '\r' }) {
		u, err := url.Parse(o)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("allowed origins must look like https://example.com")
		}
		origins = append(origins, u.Scheme+"://"+u.Host)
	}
	p.AllowedOrigins = strings.Join(origins, "\n")
	_, err := a.db.W.ExecContext(ctx, `UPDATE projects SET name=?, allowed_origins=?, doi_enabled=?, position_mode=?,
		points_per_referral=?, referral_ip_cap=?, referral_hourly_cap=?, show_position=?, counter_floor=?, from_name=?,
		reply_to=?, headline=?, description=?, site_url=?, redirect_url=?, invite_url=?,
		consent_version = CASE WHEN consent_text != ? THEN consent_version + 1 ELSE consent_version END,
		consent_text=? WHERE id=?`,
		p.Name, p.AllowedOrigins, p.DOIEnabled, p.PositionMode, p.PointsPerReferral, p.ReferralIPCap,
		p.ReferralHourlyCap, p.ShowPosition, p.CounterFloor, p.FromName, p.ReplyTo, p.Headline, p.Description,
		p.SiteURL, p.RedirectURL, p.InviteURL, p.ConsentText, p.ConsentText, p.ID)
	return err
}

func (a *App) DeleteProject(ctx context.Context, id int64) error {
	_, err := a.db.W.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	return err
}

func (p *Project) Origins() []string {
	if p.AllowedOrigins == "" {
		return nil
	}
	return strings.Split(p.AllowedOrigins, "\n")
}

// OriginAllowed reports whether a browser origin may post to this project.
// An empty allow-list accepts any origin.
func (p *Project) OriginAllowed(origin string) bool {
	list := p.Origins()
	if len(list) == 0 {
		return true
	}
	for _, o := range list {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
