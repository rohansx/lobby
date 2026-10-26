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

// emailBody renders a short transactional email as plain text and minimal HTML.
// Each paragraph is plain text; a single call-to-action link is set apart.
func emailBody(paras []string, ctaLabel, ctaURL string, footer string) (string, string) {
	var t, h strings.Builder
	h.WriteString(`<div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.55;color:#1a1a1a;max-width:520px">`)
	for _, p := range paras {
		t.WriteString(p + "\n\n")
		h.WriteString("<p>" + html.EscapeString(p) + "</p>")
	}
	if ctaURL != "" {
		t.WriteString(ctaLabel + ":\n" + ctaURL + "\n\n")
		h.WriteString(`<p style="margin:24px 0"><a href="` + html.EscapeString(ctaURL) + `" style="background:#1a1a1a;color:#fff;padding:11px 18px;border-radius:8px;text-decoration:none;display:inline-block">` +
			html.EscapeString(ctaLabel) + `</a></p><p style="font-size:13px;color:#666">Or paste this link into your browser:<br>` + html.EscapeString(ctaURL) + `</p>`)
	}
	if footer != "" {
		t.WriteString("--\n" + footer + "\n")
		h.WriteString(`<p style="font-size:12px;color:#888;border-top:1px solid #eee;padding-top:12px;margin-top:28px">` + footer + `</p>`)
	}
	h.WriteString("</div>")
	return t.String(), h.String()
}

func (a *App) composeConfirm(p *Project, s *Signup) *Message {
	link := a.cfg.BaseURL + "/s/" + a.StatusToken(s) + "?confirm=1"
	text, htm := emailBody([]string{
		"Thanks for joining the " + p.Name + " waitlist.",
		"Confirm your email to lock in your spot. The link also shows your place in line and a referral link that moves you up.",
	}, "Confirm my spot", link, "You got this because someone entered this address on the "+p.Name+" waitlist. If it wasn't you, ignore this email and you won't hear from us again.")
	return &Message{From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: "Confirm your spot on the " + p.Name + " waitlist", Text: text, HTML: htm}
}

func (a *App) composeStatus(p *Project, s *Signup) *Message {
	link := a.cfg.BaseURL + "/s/" + a.StatusToken(s)
	text, htm := emailBody([]string{
		"You're on the " + p.Name + " waitlist.",
		"Your status page shows your place in line and your referral link. Keep this email to get back to it.",
	}, "See my spot", link, "You got this because this address is on the "+p.Name+" waitlist.")
	return &Message{From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: "Your spot on the " + p.Name + " waitlist", Text: text, HTML: htm}
}

func (a *App) composeInvite(p *Project, s *Signup, inv *Invite) *Message {
	link := a.InviteLink(p, s, inv.ID)
	text, htm := emailBody([]string{
		"Good news: you're in. Your spot on the " + p.Name + " waitlist just came up.",
		"Your invite code is " + inv.Code + ". It only works with this email address.",
	}, "Get started", link, "You got this because you joined the "+p.Name+" waitlist.")
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
	text, htm := emailBody(paras, "", "", "You're getting this because you joined the "+p.Name+" waitlist.")
	text += "Unsubscribe: " + unsub + "\n"
	htm = strings.TrimSuffix(htm, "</div>") + `<p style="font-size:12px"><a href="` + html.EscapeString(unsub) + `" style="color:#888">Unsubscribe</a></p></div>`
	return &Message{
		From: a.fromAddress(p), To: s.Email, ReplyTo: p.ReplyTo, Subject: subject, Text: text, HTML: htm,
		Headers: map[string]string{
			"List-Unsubscribe":      "<" + unsub + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}
}
