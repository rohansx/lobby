package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"unicode"
)

// Launch kit assets: logos, screenshots and the like, stored in SQLite so the
// backup covers them. The type is sniffed from the bytes, never taken from the
// upload, and only a few image types and PDF are accepted.

const (
	maxAssetBytes       = 5 << 20
	maxAssetsPerProject = 20
)

var assetKinds = map[string]bool{"logo": true, "screenshot": true, "banner": true, "demo": true, "other": true}

var assetMimes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true, "application/pdf": true}

type Asset struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Filename  string `json:"filename"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
}

// sniffMime returns the allowed type of data, or "".
func sniffMime(data []byte) string {
	m, _, _ := strings.Cut(http.DetectContentType(data), ";")
	if assetMimes[m] {
		return m
	}
	return ""
}

// cleanFilename keeps a short, header-safe base name.
func cleanFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteRune('_')
		}
	}
	s := []rune(strings.Trim(b.String(), "."))
	if len(s) > 100 {
		s = s[len(s)-100:]
	}
	if len(s) == 0 {
		return "file"
	}
	return string(s)
}

func (a *App) AddAsset(ctx context.Context, projectID int64, kind, filename string, data []byte) (*Asset, error) {
	if !assetKinds[kind] {
		return nil, &ValidationError{"Kind must be logo, screenshot, banner, demo or other."}
	}
	if len(data) == 0 {
		return nil, &ValidationError{"The file is empty."}
	}
	if len(data) > maxAssetBytes {
		return nil, &ValidationError{"Files can be up to 5 MB."}
	}
	m := sniffMime(data)
	if m == "" {
		return nil, &ValidationError{"Upload a PNG, JPEG, WebP, GIF or PDF file."}
	}
	as := &Asset{Kind: kind, Filename: cleanFilename(filename), Mime: m, Size: int64(len(data)), CreatedAt: now()}
	// One statement, so the count and the insert can't race.
	res, err := a.db.W.ExecContext(ctx, `INSERT INTO assets (project_id, kind, filename, mime, size, data, created_at)
		SELECT ?, ?, ?, ?, ?, ?, ? WHERE (SELECT COUNT(*) FROM assets WHERE project_id = ?) < ?`,
		projectID, as.Kind, as.Filename, as.Mime, as.Size, data, as.CreatedAt, projectID, maxAssetsPerProject)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, &ValidationError{fmt.Sprintf("A product can have up to %d files. Delete one first.", maxAssetsPerProject)}
	}
	as.ID, _ = res.LastInsertId()
	return as, nil
}

func (a *App) Assets(ctx context.Context, projectID int64) ([]Asset, error) {
	rows, err := a.db.R.QueryContext(ctx, `SELECT id, kind, filename, mime, size, created_at FROM assets WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var as Asset
		if err := rows.Scan(&as.ID, &as.Kind, &as.Filename, &as.Mime, &as.Size, &as.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, as)
	}
	return out, rows.Err()
}

func (a *App) AssetData(ctx context.Context, projectID, id int64) (*Asset, []byte, error) {
	as := &Asset{}
	var data []byte
	err := a.db.R.QueryRowContext(ctx, `SELECT id, kind, filename, mime, size, created_at, data FROM assets WHERE id = ? AND project_id = ?`, id, projectID).
		Scan(&as.ID, &as.Kind, &as.Filename, &as.Mime, &as.Size, &as.CreatedAt, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	return as, data, err
}

func (a *App) DeleteAsset(ctx context.Context, projectID, id int64) error {
	res, err := a.db.W.ExecContext(ctx, `DELETE FROM assets WHERE id = ? AND project_id = ?`, id, projectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// hasCampaign reports whether the account has a Launch Pack campaign that isn't cancelled.
func (a *App) hasCampaign(ctx context.Context, ownerID int64) (bool, error) {
	var ok bool
	err := a.db.R.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM campaigns WHERE owner_id = ? AND status != ?)`, ownerID, statusCancelled).Scan(&ok)
	return ok, err
}
