package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps two pools over the same SQLite file: a single writer connection
// (so writes never contend for the lock) and a pool of readers for the dashboard.
type Store struct {
	W *sql.DB
	R *sql.DB
}

func OpenStore(dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, "lobby.db")
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	w, err := sql.Open("sqlite", dsn+"&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	r, err := sql.Open("sqlite", dsn+"&mode=ro")
	if err != nil {
		return nil, err
	}
	r.SetMaxOpenConns(8)
	s := &Store{W: w, R: r}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() {
	s.R.Close()
	s.W.Close()
}

var migrations = []string{
	`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	CREATE TABLE admins (
		id INTEGER PRIMARY KEY, email TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, created_at INTEGER NOT NULL);
	CREATE TABLE sessions (
		token_hash TEXT PRIMARY KEY, admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE, expires_at INTEGER NOT NULL);
	CREATE TABLE projects (
		id INTEGER PRIMARY KEY,
		slug TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		public_key TEXT UNIQUE NOT NULL,
		secret_key_hash TEXT NOT NULL,
		allowed_origins TEXT NOT NULL DEFAULT '',
		doi_enabled INTEGER NOT NULL DEFAULT 1,
		position_mode TEXT NOT NULL DEFAULT 'points',
		points_per_referral INTEGER NOT NULL DEFAULT 10,
		referral_ip_cap INTEGER NOT NULL DEFAULT 3,
		referral_hourly_cap INTEGER NOT NULL DEFAULT 20,
		show_position INTEGER NOT NULL DEFAULT 1,
		counter_floor INTEGER NOT NULL DEFAULT 50,
		from_name TEXT NOT NULL DEFAULT '',
		reply_to TEXT NOT NULL DEFAULT '',
		headline TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		site_url TEXT NOT NULL DEFAULT '',
		redirect_url TEXT NOT NULL DEFAULT '',
		invite_url TEXT NOT NULL DEFAULT '',
		consent_text TEXT NOT NULL DEFAULT 'I agree to receive emails about this waitlist.',
		consent_version INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL);
	CREATE TABLE signups (
		id INTEGER PRIMARY KEY,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		email TEXT NOT NULL,
		email_norm TEXT NOT NULL,
		status TEXT NOT NULL,
		base_position INTEGER NOT NULL,
		points INTEGER NOT NULL DEFAULT 0,
		referral_code TEXT UNIQUE NOT NULL,
		referred_by INTEGER REFERENCES signups(id) ON DELETE SET NULL,
		referral_credited INTEGER NOT NULL DEFAULT 0,
		source_url TEXT NOT NULL DEFAULT '',
		referrer TEXT NOT NULL DEFAULT '',
		utm_source TEXT NOT NULL DEFAULT '',
		utm_medium TEXT NOT NULL DEFAULT '',
		utm_campaign TEXT NOT NULL DEFAULT '',
		utm_term TEXT NOT NULL DEFAULT '',
		utm_content TEXT NOT NULL DEFAULT '',
		consent_version INTEGER NOT NULL DEFAULT 0,
		ip_hmac TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		confirmed_at INTEGER,
		invited_at INTEGER,
		joined_at INTEGER,
		removed_at INTEGER,
		unsubscribed_at INTEGER,
		last_status_email_at INTEGER,
		UNIQUE(project_id, email_norm));
	CREATE INDEX signups_line ON signups(project_id, status, base_position);
	CREATE INDEX signups_referred_by ON signups(referred_by);
	CREATE TABLE invites (
		id INTEGER PRIMARY KEY,
		signup_id INTEGER NOT NULL REFERENCES signups(id) ON DELETE CASCADE,
		token_hash TEXT UNIQUE NOT NULL,
		code TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		expires_at INTEGER,
		redeemed_at INTEGER,
		revoked_at INTEGER);
	CREATE INDEX invites_signup ON invites(signup_id);
	CREATE TABLE webhooks (
		id INTEGER PRIMARY KEY,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		url TEXT NOT NULL,
		secret TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'standard',
		events TEXT NOT NULL DEFAULT '*',
		active INTEGER NOT NULL DEFAULT 1,
		disabled_reason TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL);
	CREATE TABLE deliveries (
		id INTEGER PRIMARY KEY,
		webhook_id INTEGER NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
		msg_id TEXT NOT NULL,
		event TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		attempts INTEGER NOT NULL DEFAULT 0,
		next_at INTEGER NOT NULL,
		response_code INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL);
	CREATE INDEX deliveries_due ON deliveries(status, next_at);
	CREATE TABLE jobs (
		id INTEGER PRIMARY KEY,
		kind TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued',
		attempts INTEGER NOT NULL DEFAULT 0,
		run_at INTEGER NOT NULL,
		last_error TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL);
	CREATE INDEX jobs_due ON jobs(status, run_at);
	CREATE TABLE suppressions (
		project_id INTEGER NOT NULL,
		email_hmac TEXT NOT NULL,
		reason TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		PRIMARY KEY (project_id, email_hmac));
	CREATE TABLE broadcasts (
		id INTEGER PRIMARY KEY,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		subject TEXT NOT NULL,
		body TEXT NOT NULL,
		audience TEXT NOT NULL,
		recipients INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL);`,
	`ALTER TABLE projects ADD COLUMN notify_email TEXT NOT NULL DEFAULT '';
	UPDATE projects SET notify_email = COALESCE((SELECT email FROM admins ORDER BY id LIMIT 1), '');`,
	`ALTER TABLE admins ADD COLUMN verified_at INTEGER;
	UPDATE admins SET verified_at = created_at;
	ALTER TABLE admins ADD COLUMN plan TEXT NOT NULL DEFAULT 'free';
	ALTER TABLE admins ADD COLUMN plan_interval TEXT NOT NULL DEFAULT '';
	ALTER TABLE admins ADD COLUMN sub_status TEXT NOT NULL DEFAULT '';
	ALTER TABLE admins ADD COLUMN period_end INTEGER;
	ALTER TABLE admins ADD COLUMN cancel_at_period_end INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE admins ADD COLUMN polar_customer_id TEXT NOT NULL DEFAULT '';
	ALTER TABLE admins ADD COLUMN polar_subscription_id TEXT NOT NULL DEFAULT '';
	ALTER TABLE admins ADD COLUMN limit_mailed_at INTEGER;
	ALTER TABLE projects ADD COLUMN owner_id INTEGER REFERENCES admins(id) ON DELETE CASCADE;
	UPDATE projects SET owner_id = (SELECT id FROM admins ORDER BY id LIMIT 1);
	CREATE INDEX projects_owner ON projects(owner_id);
	CREATE TABLE auth_tokens (
		token_hash TEXT PRIMARY KEY,
		admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
		purpose TEXT NOT NULL,
		expires_at INTEGER NOT NULL);
	CREATE TABLE email_usage (
		admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
		month TEXT NOT NULL,
		sent INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (admin_id, month));
	CREATE TABLE billing_events (id TEXT PRIMARY KEY, created_at INTEGER NOT NULL);`,
}

func (s *Store) migrate() error {
	if _, err := s.W.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var v int
	err := s.W.QueryRow(`SELECT version FROM schema_version`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.W.Exec(`INSERT INTO schema_version VALUES (0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := s.W.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`UPDATE schema_version SET version = ?`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.W.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.W.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// EnsureSecret returns the instance secret used for token signing and email/IP
// HMACs, generating and persisting it on first boot.
func (s *Store) EnsureSecret(ctx context.Context) ([]byte, error) {
	v, err := s.Setting(ctx, "secret")
	if err != nil {
		return nil, err
	}
	if v == "" {
		v = randHex(32)
		if err := s.SetSetting(ctx, "secret", v); err != nil {
			return nil, err
		}
	}
	return hex.DecodeString(v)
}

func now() int64 { return time.Now().Unix() }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

const codeAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func randCode(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}
