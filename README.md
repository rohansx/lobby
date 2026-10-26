# Lobby

A self-hosted waitlist for every project you have, in one binary.

One install runs the waitlist for convox, cloakpipe and whatever ships next: an embed widget, a plain HTML form, a hosted page, double opt-in, place in line, referrals, invite waves, signed webhooks and a CLI. Signups live in one SQLite file on your box; nothing leaves it except email.

## Run it

```sh
docker compose up -d          # then open http://localhost:8080 and create the admin
```

or without Docker:

```sh
go build -o lobby ./cmd/lobby
LOBBY_BASE_URL=https://wait.example.com ./lobby serve
```

The first visit goes to `/setup`, which creates the admin account and then disappears. Create a project in the dashboard and copy one of its three snippets.

| Variable | Purpose |
| --- | --- |
| `LOBBY_BASE_URL` | Public URL of the instance (the one setting you must get right) |
| `LOBBY_FROM_EMAIL` | Sender address, ideally on a sending subdomain (`waitlist@mail.example.com`) |
| `RESEND_API_KEY` or `SMTP_URL` | Email provider. `SMTP_URL` is `smtp://user:pass@host:587` or `smtps://…:465`. With neither set, emails are written to the log |
| `RESEND_WEBHOOK_SECRET` | Point Resend's bounce and complaint webhook at `/v1/hooks/resend`; those addresses are suppressed |
| `LITESTREAM_REPLICA_URL` | e.g. `s3://bucket/lobby`: continuous backup, restored automatically on a fresh volume |
| `LOBBY_DATA` | Data directory (default `./data`, `/data` in the image) |
| `LOBBY_TRUST_PROXY` | `0` if Lobby is exposed directly instead of behind a reverse proxy |

**Deliverability matters more than any feature.** Before launch, add the SPF, DKIM and DMARC records your provider gives you for the sending subdomain, then sign up from a fresh inbox and check that the confirmation lands.

## Add it to a site

Script widget (under 2 KB gzipped, shadow DOM, reads `?ref=` and UTM tags):

```html
<div id="waitlist"></div>
<script defer src="https://wait.example.com/v1/wl.js" data-project="pk_…" data-target="#waitlist"></script>
```

Options: `data-button`, `data-placeholder`, `data-success`, `data-counter`. Theme it with CSS variables on the host element: `--wl-accent`, `--wl-accent-ink`, `--wl-bg`, `--wl-ink`, `--wl-border`, `--wl-radius`, `--wl-muted`, `--wl-error`. It fires a `lobby:signup` event.

Plain form, no JavaScript:

```html
<form method="post" action="https://wait.example.com/v1/w/pk_…/signup">
  <input type="email" name="email" required>
  <input type="text" name="website" tabindex="-1" autocomplete="off" style="position:absolute;left:-9999px">
  <button>Join the waitlist</button>
</form>
```

Or share the hosted page at `/w/{slug}`.

## How it works

1. A signup is deduped per project on the normalized address (Gmail dots and `+tags` fold together) and stores the source URL, referrer, UTM tags and `?ref=` code.
2. With double opt-in (the default) the confirmation link opens the person's status page: place in line, people behind them and their referral link. Confirming and sharing happen in one step.
3. A referrer gets credit only after their friend confirms, capped per IP address (3) and per hour (20). In points mode each credited referral moves the referrer up 10 places, which is configurable.
4. **Invite next N** emails the front of the line a magic link and a code bound to their email. Your product calls `POST /v1/invites/redeem` with the `?wl=` token (or email + code), or `POST /v1/signups/{id}/joined`.
5. Every change fires a [Standard Webhooks](https://www.standardwebhooks.com/) event, written in the same transaction, retried for about a day and redeliverable from the dashboard. A Slack preset is included.

Spam defenses that need no keys: honeypot, dwell time, per-IP rate limit, a ~10k-domain disposable list, MX check, and identical responses whether or not an address is already on the list.

Privacy: IPs are stored only as keyed hashes and cleared after 30 days, there are no cookies on public pages, consent text is versioned per signup, and hard delete leaves a suppression hash so a bot can't re-add the address.

## CLI and API

The binary is also the CLI (the Docker image links it as `wl`):

```sh
export LOBBY_URL=https://wait.example.com LOBBY_KEY=sk_…   # project secret key
lobby stats
lobby signups --status confirmed
lobby invite --next 50
lobby export --format csv > list.csv
```

The full API is in [`/llms.txt`](internal/app/llms.go) on every instance, so coding agents can operate it.

## Deploying on Dokploy

`blueprints/lobby/` holds a `docker-compose.yml` and `template.toml` for a Dokploy template. Set `LOBBY_BASE_URL` to the domain, mount `/data` and set `LITESTREAM_REPLICA_URL` for backups.

## Not yet

MCP endpoint and `openapi.json`, CSV import from GetWaitlist and Tally, export in listmonk, Kit and Mailchimp formats, Turnstile/ALTCHA, an Umami webhook preset, multiple admins, custom domains for hosted pages.

## License

MIT
