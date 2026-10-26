package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

type Message struct {
	From    string
	To      string
	ReplyTo string
	Subject string
	Text    string
	HTML    string
	Headers map[string]string
}

type Mailer interface {
	Send(ctx context.Context, m *Message) error
	Name() string
}

// ErrPermanent marks a send failure that retrying cannot fix.
var ErrPermanent = errors.New("permanent failure")

func newMailer(cfg Config, log *slog.Logger) Mailer {
	switch {
	case cfg.ResendKey != "":
		return &resendMailer{key: cfg.ResendKey, client: &http.Client{Timeout: 20 * time.Second}}
	case cfg.SMTPURL != "":
		return &smtpMailer{url: cfg.SMTPURL}
	default:
		return &logMailer{log: log}
	}
}

type logMailer struct{ log *slog.Logger }

func (l *logMailer) Name() string { return "log (no email provider configured)" }

func (l *logMailer) Send(_ context.Context, m *Message) error {
	l.log.Info("email (log driver, not sent)", "to", m.To, "subject", m.Subject, "body", m.Text)
	return nil
}

type resendMailer struct {
	key    string
	client *http.Client
}

func (r *resendMailer) Name() string { return "Resend" }

func (r *resendMailer) Send(ctx context.Context, m *Message) error {
	payload := map[string]any{"from": m.From, "to": []string{m.To}, "subject": m.Subject, "text": m.Text, "html": m.HTML}
	if m.ReplyTo != "" {
		payload["reply_to"] = m.ReplyTo
	}
	if len(m.Headers) > 0 {
		payload["headers"] = m.Headers
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 {
		return nil
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	err = fmt.Errorf("resend: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	if resp.StatusCode == 422 || resp.StatusCode == 403 || resp.StatusCode == 400 {
		return fmt.Errorf("%w: %v", ErrPermanent, err)
	}
	return err
}

type smtpMailer struct{ url string }

func (s *smtpMailer) Name() string { return "SMTP" }

func (s *smtpMailer) Send(ctx context.Context, m *Message) error {
	u, err := url.Parse(s.url)
	if err != nil {
		return fmt.Errorf("%w: bad SMTP_URL: %v", ErrPermanent, err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = map[bool]string{true: "465", false: "587"}[u.Scheme == "smtps"]
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("%w: bad from address: %v", ErrPermanent, err)
	}
	raw, err := buildMIME(m)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(host, port)
	var conn net.Conn
	d := &net.Dialer{Timeout: 15 * time.Second}
	if u.Scheme == "smtps" {
		conn, err = tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(60 * time.Second))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if u.Scheme != "smtps" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return err
			}
		}
	}
	if u.User != nil {
		pass, _ := u.User.Password()
		if err := c.Auth(smtp.PlainAuth("", u.User.Username(), pass, host)); err != nil {
			return fmt.Errorf("%w: smtp auth: %v", ErrPermanent, err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(m.To); err != nil {
		var te *textproto.Error
		if errors.As(err, &te) && te.Code >= 500 {
			return fmt.Errorf("%w: %v", ErrPermanent, err)
		}
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func buildMIME(m *Message) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	hdr("From", m.From)
	hdr("To", m.To)
	if m.ReplyTo != "" {
		hdr("Reply-To", m.ReplyTo)
	}
	hdr("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	hdr("Date", time.Now().Format(time.RFC1123Z))
	hdr("Message-ID", fmt.Sprintf("<%s@lobby>", randHex(12)))
	hdr("MIME-Version", "1.0")
	for k, v := range m.Headers {
		hdr(k, v)
	}
	hdr("Content-Type", "multipart/alternative; boundary="+mw.Boundary())
	buf.WriteString("\r\n")
	for _, part := range []struct{ ct, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.ct + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(pw)
		qp.Write([]byte(part.body))
		qp.Close()
	}
	mw.Close()
	return buf.Bytes(), nil
}

func (a *App) fromAddress(p *Project) string {
	addr := a.cfg.FromEmail
	if addr == "" {
		addr = "waitlist@localhost"
	}
	name := p.FromName
	if name == "" {
		name = p.Name
	}
	return (&mail.Address{Name: name, Address: addr}).String()
}

// email is one transactional message. render produces a plain-text part and a
// table-based HTML part that holds up in Gmail, Apple Mail and Outlook.
type email struct {
	Project   string // shown above the heading
	Preheader string // inbox preview line
	Heading   string
	Paras     []string // plain text
	Code      string   // optional, set apart in a box
	CTA, URL  string   // optional button
	Footer    string   // plain text
	Unsub     string   // optional unsubscribe URL
}

const (
	mailFont  = `-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif`
	mailInk   = "#18181b"
	mailBody  = "#3f3f46"
	mailMuted = "#71717a"
)

func (e email) render() (string, string) {
	var t, h strings.Builder
	esc := html.EscapeString

	if e.Heading != "" {
		t.WriteString(e.Heading + "\n\n")
	}
	for _, p := range e.Paras {
		t.WriteString(p + "\n\n")
	}
	if e.Code != "" {
		t.WriteString("Invite code: " + e.Code + "\n\n")
	}
	if e.URL != "" {
		t.WriteString(e.CTA + ": " + e.URL + "\n\n")
	}
	if e.Footer != "" {
		t.WriteString("--\n" + e.Footer + "\n")
	}
	if e.Unsub != "" {
		t.WriteString("Unsubscribe: " + e.Unsub + "\n")
	}

	h.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light only"><meta name="supported-color-schemes" content="light"></head>`)
	h.WriteString(`<body style="margin:0;padding:0;background:#f4f4f5;-webkit-text-size-adjust:100%">`)
	if e.Preheader != "" {
		h.WriteString(`<div style="display:none;max-height:0;overflow:hidden;opacity:0;mso-hide:all">` + esc(e.Preheader) + strings.Repeat("&#8199;&#65279;&#847; ", 40) + `</div>`)
	}
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f4f4f5"><tr><td align="center" style="padding:40px 16px">`)
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:480px;background:#ffffff;border:1px solid #e4e4e7;border-radius:12px">`)
	h.WriteString(`<tr><td style="padding:32px 32px 28px;font-family:` + mailFont + `;color:` + mailInk + `">`)
	if e.Project != "" {
		h.WriteString(`<p style="margin:0 0 24px;font-size:14px;line-height:20px;font-weight:600;color:` + mailInk + `">` + esc(e.Project) + `</p>`)
	}
	if e.Heading != "" {
		h.WriteString(`<h1 style="margin:0 0 12px;font-size:22px;line-height:28px;font-weight:600;color:` + mailInk + `">` + esc(e.Heading) + `</h1>`)
	}
	for _, p := range e.Paras {
		h.WriteString(`<p style="margin:0 0 14px;font-size:15px;line-height:24px;color:` + mailBody + `">` + esc(p) + `</p>`)
	}
	if e.Code != "" {
		h.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 6px"><tr><td style="padding:10px 16px;background:#f4f4f5;border-radius:8px;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:18px;line-height:24px;letter-spacing:2px;font-weight:600;color:` + mailInk + `">` + esc(e.Code) + `</td></tr></table>`)
	}
	if e.URL != "" {
		u := esc(e.URL)
		// Bulletproof button: the cell carries the colour so clients that
		// ignore padding on links (Outlook) still show a solid button.
		h.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:24px 0 28px"><tr><td align="center" bgcolor="` + mailInk + `" style="border-radius:8px;background:` + mailInk + `">`)
		h.WriteString(`<a href="` + u + `" target="_blank" style="display:inline-block;padding:12px 24px;border:1px solid ` + mailInk + `;border-radius:8px;font-family:` + mailFont + `;font-size:15px;line-height:20px;font-weight:600;color:#ffffff;text-decoration:none;mso-padding-alt:0">` + esc(e.CTA) + `</a>`)
		h.WriteString(`</td></tr></table>`)
		h.WriteString(`<p style="margin:0;font-size:13px;line-height:20px;color:` + mailMuted + `">Or open this link:<br><a href="` + u + `" target="_blank" style="color:` + mailMuted + `;text-decoration:underline;word-break:break-all">` + u + `</a></p>`)
	}
	h.WriteString(`</td></tr></table>`)
	if e.Footer != "" || e.Unsub != "" {
		h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:480px"><tr><td style="padding:20px 32px 0;font-family:` + mailFont + `;font-size:12px;line-height:18px;color:#a1a1aa;text-align:center">`)
		h.WriteString(esc(e.Footer))
		if e.Unsub != "" {
			if e.Footer != "" {
				h.WriteString("<br>")
			}
			h.WriteString(`<a href="` + esc(e.Unsub) + `" target="_blank" style="color:#a1a1aa;text-decoration:underline">Unsubscribe</a>`)
		}
		h.WriteString(`</td></tr></table>`)
	}
	h.WriteString(`</td></tr></table></body></html>`)
	return t.String(), h.String()
}

func (a *App) composeConfirm(p *Project, s *Signup) *Message {
	paras := []string{"Thanks for joining the " + p.Name + " waitlist. Confirm this is your address to keep your place in line."}
	if p.PositionMode == "points" {
		paras = append(paras, "You'll also get a link to share. Every friend who joins through it moves you up.")
	}
	text, htm := email{
		Project:   p.Name,
		Preheader: "One click to confirm your place on the " + p.Name + " waitlist.",
		Heading:   "Confirm your email",
		Paras:     paras,
		CTA:       "Confirm email",
		URL:       a.cfg.BaseURL + "/s/" + a.StatusToken(s) + "?confirm=1",
		Footer:    "If you didn't sign up for " + p.Name + ", ignore this email. You won't hear from us again.",
	}.render()
	return &Message{From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: "Confirm your email for " + p.Name, Text: text, HTML: htm}
}

func (a *App) composeStatus(p *Project, s *Signup) *Message {
	text, htm := email{
		Project:   p.Name,
		Preheader: "Your place in line and your referral link.",
		Heading:   "You're on the list",
		Paras:     []string{"This link shows where you are in line for " + p.Name + " and your link to share. Keep this email to find it again."},
		CTA:       "See my place",
		URL:       a.cfg.BaseURL + "/s/" + a.StatusToken(s),
		Footer:    "You're getting this because this address is on the " + p.Name + " waitlist.",
	}.render()
	return &Message{From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: "Your place on the " + p.Name + " waitlist", Text: text, HTML: htm}
}

func (a *App) composeInvite(p *Project, s *Signup, inv *Invite) *Message {
	text, htm := email{
		Project:   p.Name,
		Preheader: "Your invite to " + p.Name + " is ready.",
		Heading:   "You're in",
		Paras:     []string{"Your spot on the " + p.Name + " waitlist came up. Your invite is ready whenever you are.", "If you're asked for a code, use this one. It only works with this email address."},
		Code:      inv.Code,
		CTA:       "Get started",
		URL:       a.InviteLink(p, s, inv.ID),
		Footer:    "You're getting this because you joined the " + p.Name + " waitlist.",
	}.render()
	return &Message{From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: "You're in: your " + p.Name + " invite", Text: text, HTML: htm}
}

func (a *App) composeBroadcast(p *Project, s *Signup, subject, body string) *Message {
	unsub := a.cfg.BaseURL + "/u/" + a.UnsubscribeToken(s)
	var paras []string
	for _, para := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if para = strings.TrimSpace(para); para != "" {
			paras = append(paras, para)
		}
	}
	text, htm := email{
		Project: p.Name,
		Paras:   paras,
		Footer:  "You're getting this because you joined the " + p.Name + " waitlist.",
		Unsub:   unsub,
	}.render()
	return &Message{
		From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: subject, Text: text, HTML: htm,
		Headers: map[string]string{
			"List-Unsubscribe":      "<" + unsub + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}
}
