package app

import (
	"net/http"
	"strings"
)

const llmsTxt = `# Lobby

> Self-hosted waitlist service. One instance runs the waitlist for many projects: embed widget, plain HTML form, hosted page, double opt-in, place in line, referrals, invites, signed webhooks.

Base URL: {{BASE}}

## Public endpoints (no auth, project public key pk_... in the path)

- POST /v1/w/{pk}/signup: fields email (required), ref, source_url, referrer, utm_source, utm_medium, utm_campaign, utm_term, utm_content. Accepts JSON, urlencoded or multipart. Send Accept: application/json for a JSON reply; otherwise it answers with a 303 redirect. With double opt-in on (the default), the reply is always {"ok":true,"confirm_required":true} and the person gets an email.
- GET /v1/w/{pk}/count: public count, or null below the project's floor.
- GET /v1/wl.js: embed script. <script defer src="{{BASE}}/v1/wl.js" data-project="pk_..." data-target="#waitlist"></script>. Optional data-button, data-placeholder, data-success, data-counter. Theme with --wl-accent, --wl-accent-ink, --wl-bg, --wl-ink, --wl-border, --wl-radius, --wl-muted, --wl-error.
- GET /w/{slug}: hosted waitlist page. GET /s/{token}: a person's status page.

## Admin API (Authorization: Bearer sk_..., the project secret key)

- GET /v1/project, GET /v1/stats
- GET /v1/signups?status=&q=&cursor=&limit=  (status: pending, confirmed, invited, joined, removed)
- GET /v1/signups/{id}, DELETE /v1/signups/{id}[?hard=1]
- POST /v1/signups/{id}/joined: mark someone as having signed up for the product
- POST /v1/invites {"count": N} or {"ids": [..]}: invite the next N people in line, or specific people
- POST /v1/invites/redeem {"token": "<?wl= value>"} or {"email": "...", "code": "ABCD-EFGH"}: marks the invite used and the person joined
- GET /v1/export.csv, GET /v1/export.ndjson  (?status=all to include unconfirmed)
- GET|POST /v1/webhooks {"url","kind":"standard|slack","events":[...]}, DELETE /v1/webhooks/{id}, POST /v1/webhooks/{id}/test
- GET /v1/deliveries, POST /v1/deliveries/{id}/redeliver
- POST /v1/broadcasts {"subject","body","audience":"confirmed|invited|all"}

## Webhooks

Events: signup.created, signup.confirmed, signup.referred, signup.invited, signup.joined, signup.removed. Body: {"type","timestamp","data"}. Signed per Standard Webhooks: headers webhook-id, webhook-timestamp, webhook-signature = "v1," + base64(HMAC-SHA256(base64-decoded secret after "whsec_", id + "." + timestamp + "." + body)).

## CLI

The same binary is the CLI: LOBBY_URL={{BASE}} LOBBY_KEY=sk_... lobby stats | signups | invite --next 50 | joined ID | export | remove ID. Symlink it as "wl" to type less.
`

func (s *Server) llms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(strings.ReplaceAll(llmsTxt, "{{BASE}}", s.app.cfg.BaseURL)))
}
