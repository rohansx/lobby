<p align="center"><img src="site/logo.svg" width="64" height="64" alt="Lobby logo"></p>

<h1 align="center">Lobby</h1>

**A self-hosted waitlist for every product you're launching.** One binary, one SQLite file, no monthly bill.

Lobby gives each of your products a signup form, double opt-in, a place in line, referral links that move people up, invite waves and a dashboard to run it all. Your list stays on your server.

<p align="center">
  <img src="docs/demo.gif" alt="A visitor joins the Orbit waitlist from its landing page, confirms by email, sees their place in line, and shows up in the Lobby dashboard" width="900">
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#add-it-to-your-site">Add it to your site</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="#cli-and-api">CLI and API</a>
</p>

## Why Lobby

- **Every product, one install.** Run the waitlists for all your side projects from a single dashboard.
- **You own the list.** Signups live in one SQLite file on your box. Export to CSV or NDJSON any time.
- **Built for launches.** Referral links, a live counter, invite waves and a launch broadcast are included.
- **Small and fast.** A 14 MB binary that idles at about 20 MB of memory. The embed widget is under 2 KB gzipped.
- **Spam handled without CAPTCHAs.** A honeypot, timing checks, rate limits, a disposable-domain list and MX lookups.
- **Agent-friendly.** Every instance serves its full API at `/llms.txt`, and the binary doubles as a CLI.

## Screenshots

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/dashboard-dark.png">
  <img src="docs/dashboard.png" alt="Lobby dashboard: signups, confirm rate, signups per day, top sources and top referrers">
</picture>

<table>
  <tr>
    <td width="50%"><img src="docs/widget.png" alt="The embed widget on a product landing page"><br><sub><b>Embed widget</b> on your own landing page</sub></td>
    <td width="50%"><img src="docs/signups.png" alt="Signups list with status, place in line, referrals and source"><br><sub><b>Signups</b> with status, place in line and source</sub></td>
  </tr>
  <tr>
    <td><img src="docs/status.png" alt="Status page showing place in line and a referral link"><br><sub><b>Status page</b>: place in line and a link to share</sub></td>
    <td><img src="docs/email.png" alt="Confirmation email"><br><sub><b>Confirmation email</b> that renders in every client</sub></td>
  </tr>
</table>

## Quick start

```sh
docker run -d --name lobby -p 8080:8080 -v lobby-data:/data \
  -e LOBBY_BASE_URL=http://localhost:8080 \
  ghcr.io/rohansx/lobby:latest
```

Open http://localhost:8080. The first visit asks you to create the admin account. Then create a project and copy one of its snippets onto your site.

Without email settings, Lobby writes emails to the log, which is fine for trying it out. For real signups, set an email provider (see [Configuration](#configuration)).

<details>
<summary>Docker Compose</summary>

```sh
git clone https://github.com/rohansx/lobby && cd lobby
LOBBY_BASE_URL=https://wait.example.com RESEND_API_KEY=re_… LOBBY_FROM_EMAIL=waitlist@mail.example.com \
  docker compose up -d
```
</details>

<details>
<summary>From source</summary>

Needs Go 1.26 and Node 24.

```sh
(cd web && npm ci && npm run build)   # the dashboard, embedded into the binary
go build -o lobby ./cmd/lobby
LOBBY_BASE_URL=https://wait.example.com ./lobby serve
```
</details>

<details>
<summary>Dokploy, Coolify or any VPS</summary>

Run the image behind your reverse proxy, mount a volume at `/data` and set `LOBBY_BASE_URL` to the public URL. `blueprints/lobby/` has a `docker-compose.yml` and `template.toml` for a Dokploy template. Set `LITESTREAM_REPLICA_URL` to back the database up continuously to S3-compatible storage.
</details>

## Add it to your site

Each project gives you three ways in, all shown in the dashboard with your keys filled in.

**Script widget.** A form in a shadow DOM that won't clash with your styles. It reads `?ref=` and UTM tags from the page.

```html
<div id="waitlist"></div>
<script defer src="https://wait.example.com/v1/wl.js" data-project="pk_…" data-target="#waitlist" data-counter></script>
```

Options: `data-button`, `data-placeholder`, `data-success`, `data-counter`. Theme it with CSS variables on the container: `--wl-accent`, `--wl-accent-ink`, `--wl-bg`, `--wl-ink`, `--wl-border`, `--wl-radius`, `--wl-muted`, `--wl-error`. It fires a `lobby:signup` event you can hook analytics into.

**Plain HTML form.** No JavaScript at all.

```html
<form method="post" action="https://wait.example.com/v1/w/pk_…/signup">
  <input type="email" name="email" required>
  <input type="text" name="website" tabindex="-1" autocomplete="off" style="position:absolute;left:-9999px">
  <button>Join the waitlist</button>
</form>
```

**Hosted page.** No site yet? Share `https://wait.example.com/w/your-project`.

## How it works

1. **Signup.** Addresses are deduped per project on a normalized form (Gmail dots and `+tags` fold together). Lobby stores the page, referrer, UTM tags and referral code.
2. **Confirm.** With double opt-in (the default), the confirmation link opens the person's status page: their place in line, how many people are behind them and their referral link.
3. **Refer.** A referrer gets credit only after their friend confirms, capped per IP address and per hour. In points mode each credited referral moves the referrer up 10 places (configurable). FIFO mode is also available.
4. **Invite.** **Invite next N** emails the front of the line a magic link and a code bound to their address. Your product calls `POST /v1/invites/redeem`, or marks someone joined with `POST /v1/signups/{id}/joined`.
5. **Hear about it.** Every change fires a signed [Standard Webhooks](https://www.standardwebhooks.com/) event, retried for about a day and redeliverable from the dashboard. A Slack preset is included. Lobby also emails you each new signup, with its source and referrer.

**Privacy.** IP addresses are stored only as keyed hashes and cleared after 30 days. Public pages set no cookies. Consent text is versioned per signup. A hard delete leaves a suppression hash so a bot can't re-add the address. Broadcasts carry one-click unsubscribe headers.

## Configuration

| Variable | Purpose |
| --- | --- |
| `LOBBY_BASE_URL` | Public URL of the instance. Links in emails and snippets use it, so get this one right |
| `LOBBY_FROM_EMAIL` | Sender address, ideally on a sending subdomain (`waitlist@mail.example.com`) |
| `RESEND_API_KEY` or `SMTP_URL` | Email provider. `SMTP_URL` is `smtp://user:pass@host:587` or `smtps://…:465`. With neither set, emails go to the log |
| `RESEND_WEBHOOK_SECRET` | Point Resend's bounce and complaint webhook at `/v1/hooks/resend` and those addresses are suppressed |
| `LITESTREAM_REPLICA_URL` | e.g. `s3://bucket/lobby`, plus `LITESTREAM_ACCESS_KEY_ID` and `LITESTREAM_SECRET_ACCESS_KEY`. Continuous backup, restored automatically onto an empty volume |
| `LOBBY_DATA` | Data directory (default `./data`, `/data` in the image) |
| `LOBBY_ADDR` | Listen address (default `:8080`) |
| `LOBBY_TRUST_PROXY` | `0` if Lobby is exposed directly rather than behind a reverse proxy |
| `LOBBY_CHECK_MX` | `0` to skip the MX lookup on signup |

<details>
<summary>Cloud mode (hosting Lobby for other people)</summary>

`LOBBY_CLOUD=1` turns an instance into a multi-tenant service, which is how [Lobby Cloud](https://lobby.rohan.sh/pricing) runs. Anyone can sign up at `/signup`, confirm their email, and see only their own waitlists. Accounts get the Free, Launch or Pro limits (signups, waitlists, emails a month, webhooks and API, the "powered by Lobby" badge), and billing runs through [Polar](https://polar.sh) as merchant of record. Without `LOBBY_CLOUD`, none of this applies: one admin, every feature, no limits.

| Variable | Purpose |
| --- | --- |
| `LOBBY_CLOUD` | `1` for open signup, per-account projects and plan limits |
| `POLAR_ACCESS_TOKEN` | Polar organization access token (checkouts, customer sessions, subscriptions) |
| `POLAR_WEBHOOK_SECRET` | Secret of the Polar webhook pointed at `/v1/hooks/polar`, with the `subscription.*` events |
| `POLAR_PRODUCT_LAUNCH_MONTHLY`, `_LAUNCH_YEARLY`, `_PRO_MONTHLY`, `_PRO_YEARLY` | Polar product IDs for each plan and interval |
| `POLAR_SANDBOX` | `1` to use Polar's sandbox while testing |
</details>

Per-project settings live in the dashboard: allowed origins, double opt-in, position mode, referral caps, sender name, reply-to, the new-signup notification address, page copy and consent text.

> **Deliverability matters more than any feature.** Before launch, add the SPF, DKIM and DMARC records your provider gives you for the sending domain. Then sign up from a fresh inbox and check that the confirmation lands.

## CLI and API

The binary is also a CLI. The Docker image links it as `wl`.

```sh
export LOBBY_URL=https://wait.example.com LOBBY_KEY=sk_…   # the project's secret key
lobby stats
lobby signups --status confirmed
lobby invite --next 50
lobby export --format csv > list.csv
```

The REST API covers signups, invites, exports, webhooks and broadcasts, authenticated with the project's secret key. Every instance documents it at [`/llms.txt`](internal/app/llms.go), so coding agents can run your waitlist for you.

## Development

```sh
go run ./cmd/lobby serve              # API and public pages on :8080
cd web && npm run dev                 # dashboard with hot reload; proxies the API to :8080
go test ./...
```

The dashboard is a React app in `web/` (Vite, Tailwind, shadcn/ui) that talks to a JSON API under `/api/admin`. `npm run build` writes it into `internal/app/web/dist`, which is embedded into the binary, so production stays one file. Public pages (hosted page, status page, unsubscribe) are server-rendered and work without JavaScript.

## Roadmap

- MCP endpoint and `openapi.json`
- CSV import from other waitlist tools
- Export in listmonk, Kit and Mailchimp formats
- Optional Turnstile or ALTCHA challenge
- Multiple admins
- Custom domains for hosted pages

Issues and pull requests are welcome.

## License

[MIT](LICENSE)
