package app

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"net"
	"net/mail"
	"strings"
	"sync"
	"time"
)

//go:embed web/disposable.txt
var disposableList string

type spamGuard struct {
	disposable map[string]struct{}
	checkMX    bool

	mu      sync.Mutex
	mxCache map[string]mxEntry
	windows map[string][]time.Time
}

type mxEntry struct {
	ok      bool
	expires time.Time
}

func newSpamGuard(checkMX bool) *spamGuard {
	g := &spamGuard{
		disposable: map[string]struct{}{},
		checkMX:    checkMX,
		mxCache:    map[string]mxEntry{},
		windows:    map[string][]time.Time{},
	}
	sc := bufio.NewScanner(strings.NewReader(disposableList))
	for sc.Scan() {
		d := strings.TrimSpace(sc.Text())
		if d != "" && !strings.HasPrefix(d, "#") {
			g.disposable[strings.ToLower(d)] = struct{}{}
		}
	}
	go g.sweep()
	return g
}

// Allow is a sliding-window limiter: at most n events per key within window.
func (g *spamGuard) Allow(key string, n int, window time.Duration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	cutoff := time.Now().Add(-window)
	ts := g.windows[key]
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= n {
		g.windows[key] = ts
		return false
	}
	g.windows[key] = append(ts, time.Now())
	return true
}

func (g *spamGuard) sweep() {
	for range time.Tick(10 * time.Minute) {
		g.mu.Lock()
		cutoff := time.Now().Add(-24 * time.Hour)
		for k, ts := range g.windows {
			if len(ts) == 0 || ts[len(ts)-1].Before(cutoff) {
				delete(g.windows, k)
			}
		}
		for k, e := range g.mxCache {
			if time.Now().After(e.expires) {
				delete(g.mxCache, k)
			}
		}
		g.mu.Unlock()
	}
}

func (g *spamGuard) IsDisposable(domain string) bool {
	domain = strings.ToLower(domain)
	for {
		if _, ok := g.disposable[domain]; ok {
			return true
		}
		i := strings.IndexByte(domain, '.')
		if i < 0 || !strings.Contains(domain[i+1:], ".") {
			return false
		}
		domain = domain[i+1:]
	}
}

// HasMX reports whether the domain can receive mail. DNS failures other than
// "no such host" count as yes so a flaky resolver never blocks real people.
func (g *spamGuard) HasMX(ctx context.Context, domain string) bool {
	if !g.checkMX {
		return true
	}
	domain = strings.ToLower(domain)
	g.mu.Lock()
	e, ok := g.mxCache[domain]
	g.mu.Unlock()
	if ok && time.Now().Before(e.expires) {
		return e.ok
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result := true
	mxs, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			// No MX record; RFC 5321 falls back to the A record.
			addrs, aerr := net.DefaultResolver.LookupHost(ctx, domain)
			result = aerr == nil && len(addrs) > 0
			if aerr != nil && !(errors.As(aerr, &dnsErr) && dnsErr.IsNotFound) {
				result = true
			}
		}
	} else if len(mxs) == 1 && mxs[0].Host == "." {
		result = false // null MX, RFC 7505
	}
	g.mu.Lock()
	g.mxCache[domain] = mxEntry{ok: result, expires: time.Now().Add(time.Hour)}
	g.mu.Unlock()
	return result
}

// parseEmail validates an address and returns it trimmed plus its domain.
func parseEmail(s string) (string, string, error) {
	s = strings.TrimSpace(s)
	if len(s) > 254 || s == "" {
		return "", "", errors.New("Please enter a valid email address.")
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || addr.Name != "" {
		return "", "", errors.New("Please enter a valid email address.")
	}
	at := strings.LastIndexByte(s, '@')
	domain := strings.ToLower(s[at+1:])
	if !strings.Contains(domain, ".") {
		return "", "", errors.New("Please enter a valid email address.")
	}
	return s, domain, nil
}

// normalizeEmail folds the address variants that reach the same inbox, so
// dedupe and referral caps cannot be beaten with dots or +tags.
func normalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return email
	}
	local, domain := email[:at], email[at+1:]
	if i := strings.IndexByte(local, '+'); i >= 0 {
		local = local[:i]
	}
	if domain == "googlemail.com" {
		domain = "gmail.com"
	}
	if domain == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}

func (a *App) hmacHex(purpose, value string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(purpose + ":" + value))
	return hex.EncodeToString(m.Sum(nil))
}

func (a *App) emailHMAC(email string) string { return a.hmacHex("email", normalizeEmail(email)) }

// ipHMAC is a keyed hash of the client IP. It is personal data under GDPR, so
// the worker clears it after 30 days.
func (a *App) ipHMAC(ip string) string {
	if ip == "" {
		return ""
	}
	return a.hmacHex("ip", ip)[:32]
}
