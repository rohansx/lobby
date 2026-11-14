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
	// Feedback summaries, duplicate suggestions and the digest overview use
	// Claude when a key is set. Without one, those parts are simply off.
	AnthropicKey string // ANTHROPIC_API_KEY
	AIModel      string // LOBBY_AI_MODEL, default claude-opus-5-5
	// Voice input in the feedback widget: Sarvam AI transcribes and translates
	// to English. Without a key the widget has no mic button.
	SarvamKey string // SARVAM_API_KEY

	// Cloud mode turns one instance into a hosted service: anyone can sign up,
	// each account sees only its own projects, and plans set the limits.
	Cloud     bool        // LOBBY_CLOUD
	Polar     PolarConfig // POLAR_*, billing for cloud mode
	Operators []string    // LOBBY_OPERATORS, comma-separated emails that can open the operator page

	// Sign in with Google and GitHub; each is on when both of its values are set.
	GoogleClientID, GoogleClientSecret string // GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET
	GitHubClientID, GitHubClientSecret string // GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET
}

type PolarConfig struct {
	Token         string // POLAR_ACCESS_TOKEN, an organization access token
	WebhookSecret string // POLAR_WEBHOOK_SECRET
	Sandbox       bool   // POLAR_SANDBOX=1 uses sandbox-api.polar.sh
	APIURL        string // POLAR_API_URL overrides both, for tests
	LaunchPack    string // POLAR_PRODUCT_LAUNCH_PACK, the one-time Launch Pack product
	// Products maps "launch_month" etc. to the legacy subscription products
	// (POLAR_PRODUCT_LAUNCH_MONTHLY etc.). Only existing subscribers use them:
	// no new subscriptions are sold, but their webhooks still need the mapping.
	Products map[string]string
}

// Enabled reports whether the Launch Pack can be bought.
func (p PolarConfig) Enabled() bool { return p.Token != "" && p.LaunchPack != "" }

func ConfigFromEnv() Config {
	c := Config{
		Addr:               env("LOBBY_ADDR", ":8080"),
		BaseURL:            strings.TrimRight(env("LOBBY_BASE_URL", "http://localhost:8080"), "/"),
		DataDir:            env("LOBBY_DATA", "./data"),
		TrustProxy:         env("LOBBY_TRUST_PROXY", "1") != "0",
		FromEmail:          env("LOBBY_FROM_EMAIL", ""),
		SMTPURL:            env("SMTP_URL", ""),
		ResendKey:          env("RESEND_API_KEY", ""),
		ResendHook:         env("RESEND_WEBHOOK_SECRET", ""),
		CheckMX:            env("LOBBY_CHECK_MX", "1") != "0",
		AnthropicKey:       env("ANTHROPIC_API_KEY", ""),
		AIModel:            env("LOBBY_AI_MODEL", "claude-opus-5-5"),
		SarvamKey:          env("SARVAM_API_KEY", ""),
		Cloud:              env("LOBBY_CLOUD", "") == "1",
		Operators:          strings.FieldsFunc(env("LOBBY_OPERATORS", ""), func(r rune) bool { return r == ',' || r == ' ' }),
		GoogleClientID:     env("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: env("GOOGLE_CLIENT_SECRET", ""),
		GitHubClientID:     env("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
		Polar: PolarConfig{
			Token:         env("POLAR_ACCESS_TOKEN", ""),
			WebhookSecret: env("POLAR_WEBHOOK_SECRET", ""),
			Sandbox:       env("POLAR_SANDBOX", "") == "1",
			APIURL:        strings.TrimRight(env("POLAR_API_URL", ""), "/"),
			LaunchPack:    env("POLAR_PRODUCT_LAUNCH_PACK", ""),
			Products:      map[string]string{},
		},
	}
	for _, plan := range []string{"launch", "pro"} {
		for interval, suffix := range map[string]string{"month": "MONTHLY", "year": "YEARLY"} {
			if id := env("POLAR_PRODUCT_"+strings.ToUpper(plan)+"_"+suffix, ""); id != "" {
				c.Polar.Products[plan+"_"+interval] = id
			}
		}
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
	oauth  map[string]*OAuthProvider
	ai     aiJSON    // nil when no Anthropic key is configured
	voice  voiceText // nil when no Sarvam key is configured
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
	a.oauth = oauthProviders(cfg)
	a.ai = newClaude(cfg)
	a.voice = newSarvam(cfg)
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
