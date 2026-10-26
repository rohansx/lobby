package app

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const pbkdf2Iter = 600_000

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	rand.Read(salt)
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func checkPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func (a *App) HasAdmin(ctx context.Context) bool {
	var n int
	a.db.W.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n > 0
}

// CreateFirstAdmin only succeeds while no admin exists, which is what makes /setup safe to expose.
func (a *App) CreateFirstAdmin(ctx context.Context, email, password string) error {
	email = strings.TrimSpace(email)
	if _, _, err := parseEmail(email); err != nil {
		return err
	}
	if len(password) < 10 {
		return errors.New("Use a password of at least 10 characters.")
	}
	h, err := hashPassword(password)
	if err != nil {
		return err
	}
	res, err := a.db.W.ExecContext(ctx, `INSERT INTO admins (email, password_hash, created_at)
		SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM admins)`, email, h, now())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("setup is already complete")
	}
	return nil
}

const sessionTTL = 30 * 86400

// Login returns a session token for the admin cookie.
func (a *App) Login(ctx context.Context, email, password string) (string, error) {
	var id int64
	var h string
	err := a.db.W.QueryRowContext(ctx, `SELECT id, password_hash FROM admins WHERE email = ? COLLATE NOCASE`, strings.TrimSpace(email)).Scan(&id, &h)
	if err != nil || !checkPassword(h, password) {
		return "", errors.New("Wrong email or password.")
	}
	tok := randHex(32)
	_, err = a.db.W.ExecContext(ctx, `INSERT INTO sessions (token_hash, admin_id, expires_at) VALUES (?, ?, ?)`, hashKey(tok), id, now()+sessionTTL)
	return tok, err
}

func (a *App) SessionAdmin(ctx context.Context, tok string) (int64, bool) {
	var id int64
	err := a.db.W.QueryRowContext(ctx, `SELECT admin_id FROM sessions WHERE token_hash = ? AND expires_at > ?`, hashKey(tok), now()).Scan(&id)
	return id, err == nil
}

func (a *App) Logout(ctx context.Context, tok string) {
	a.db.W.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashKey(tok))
}

func (a *App) AdminEmail(ctx context.Context, id int64) string {
	var email string
	a.db.R.QueryRowContext(ctx, `SELECT email FROM admins WHERE id = ?`, id).Scan(&email)
	return email
}
