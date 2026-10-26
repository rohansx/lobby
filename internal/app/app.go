package app

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Addr       string // LOBBY_ADDR, default :8080
	BaseURL    string // LOBBY_BASE_URL, the public URL of this instance
	DataDir    string // LOBBY_DATA, default ./data
	TrustProxy bool   // LOBBY_TRUST_PROXY, take client IP from X-Forwarded-For (default on)
	FromEmail  string // LOBBY_FROM_EMAIL, e.g. waitlist@mail.example.com
	SMTPURL    string // SMTP_URL, smtp://user:pass@host:587 or smtps://...:465
	ResendKey  string // RESEND_API_KEY
	ResendHook string // RESEND_WEBHOOK_SECRET, for bounce and complaint suppression
	CheckMX    bool   // LOBBY_CHECK_MX, default on
}

func ConfigFromEnv() Config {
	c := Config{
		Addr:       env("LOBBY_ADDR", ":8080"),
		BaseURL:    strings.TrimRight(env("LOBBY_BASE_URL", "http://localhost:8080"), "/"),
		DataDir:    env("LOBBY_DATA", "./data"),
		TrustProxy: env("LOBBY_TRUST_PROXY", "1") != "0",
		FromEmail:  env("LOBBY_FROM_EMAIL", ""),
		SMTPURL:    env("SMTP_URL", ""),
		ResendKey:  env("RESEND_API_KEY", ""),
		ResendHook: env("RESEND_WEBHOOK_SECRET", ""),
		CheckMX:    env("LOBBY_CHECK_MX", "1") != "0",
	}
	return c
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type App struct {
	cfg    Config
	db     *Store
	secret []byte
	mailer Mailer
	log    *slog.Logger
	spam   *spamGuard
	wake   chan struct{}
}

func New(ctx context.Context, cfg Config) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	db, err := OpenStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	secret, err := db.EnsureSecret(ctx)
	if err != nil {
		return nil, err
	}
	a := &App{
		cfg:    cfg,
		db:     db,
		secret: secret,
		log:    slog.Default(),
		wake:   make(chan struct{}, 1),
	}
	a.mailer = newMailer(cfg, a.log)
	a.spam = newSpamGuard(cfg.CheckMX)
	return a, nil
}

func (a *App) Close() { a.db.Close() }

func (a *App) Config() Config { return a.cfg }

// kick wakes the background worker so queued jobs run without waiting for the next tick.
func (a *App) kick() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}
