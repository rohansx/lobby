# Claimable accounts: design

Status: built on branch `rohansx/claim-acc-feat`. Differences from the draft are noted inline.

## Why

A founder doing outbound sales builds a ready-made account for a prospect inside their own product (data loaded, settings done), then wants the prospect to take it over. Today that means inventing a placeholder email for the account and writing claim links, expiry, email verification and a claim page in every product.

Lobby already issues signed links, verifies email addresses, hands people off to the product with one API call and reports it by webhook. Claiming an account is the same handshake with one extra fact: which account the link belongs to.

## What it does

1. The founder creates a **claim** in Lobby for an account that already exists in their product, identified by `account_ref` (the product's own account id, opaque to Lobby). Optional: a label ("Acme, Priya") and the prospect's email.
2. Lobby returns a **claim link**: `{LOBBY_URL}/c/{token}`. The founder sends it however they like (email, DM, a slide).
3. The prospect opens it and sees a hosted page: "Your {Project} account is ready". They enter their email. Lobby sends a confirmation email.
4. They click the link in that email and press Continue (built: a button page, so mail scanners that prefetch links can't spend the code; the email confirmed there is the one redeem returns). Lobby marks the claim verified and redirects to the project's **Invite URL** (else its Site URL) with `?claim={code}`. (Built: the existing Invite URL is reused instead of a new Claim URL setting; products tell `?claim=` from `?wl=`.) The code is single-use and valid for 15 minutes.
5. The product calls `POST /v1/claims/redeem` with the code and its secret key. Lobby answers with `account_ref` and the verified email. The product sets that email on the account and logs the person in.

The product's whole share of the work is step 5:

```ts
// GET /claim?claim=...
const res = await fetch(`${LOBBY_URL}/v1/claims/redeem`, {
  method: "POST",
  headers: { Authorization: `Bearer ${LOBBY_SECRET_KEY}`, "Content-Type": "application/json" },
  body: JSON.stringify({ code: req.query.claim }),
})
if (!res.ok) return showExpired()
const { account_ref, email } = await res.json()
await db.accounts.update(account_ref, { email })   // replaces the placeholder
await startSession(account_ref)                    // then ask them to set a password
```

## Decisions (my calls, open to change)

- **Lobby always verifies the email at claim time.** A link sent over DM can be forwarded, so the link alone never transfers an account. One flow covers both "I know their email" and "I only have their LinkedIn".
- **An email given at creation locks the claim to it.** The claim page shows that address and only it can claim. No email given means whoever holds the link can claim with their own address.
- **The product keeps the account.** Lobby never stores product credentials or logs anyone in. It proves "this email owns the right to account X" and nothing more.
- **Redeem is server-to-server with the secret key.** The browser only ever carries the short-lived code.
- **A claimed prospect becomes a joined signup** (source `claim`), the same way testers are stored. They show up in Signups, count toward the plan's signup limit at that moment, and can be given the feedback widget. The redeem response includes `reporter_token`, like invite redeem does.
- **Claims expire after 30 days** unless `expires_in` says otherwise. One open claim per `account_ref` per project; creating another revokes the first.

## Approaches considered

1. **A separate `claims` table and flow (chosen).** Clear model, nothing existing changes, about one Go file plus a dashboard page.
2. **Extend invites with an `account_ref` column.** Less schema, but `invites.signup_id` is required and an invite assumes the person already confirmed an email on the waitlist. A prospect has done neither, so every invite query would grow a special case.
3. **Product-side library instead of a Lobby feature.** The founder would still host the claim page and send the email from each product, which is the work this is meant to remove.

## Data

Migration: one table, one project column.

```sql
CREATE TABLE claims (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  account_ref TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  email_lock TEXT NOT NULL DEFAULT '',      -- normalized; '' means anyone with the link
  pending_email TEXT NOT NULL DEFAULT '',   -- entered on the claim page, not yet confirmed
  code_hash TEXT NOT NULL DEFAULT '',       -- the redeem code, set when the email is confirmed
  code_expires_at INTEGER,
  signup_id INTEGER REFERENCES signups(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  opened_at INTEGER,
  verified_at INTEGER,
  claimed_at INTEGER,
  revoked_at INTEGER);
CREATE INDEX claims_project ON claims(project_id, account_ref);
```

The link token is derived from the claim id with `tokenSig("claim", id)`, as `inviteToken` does, so the dashboard can show the link again without storing it. The email-confirmation token is signed the same way over claim id plus pending email.

## Surface

| Where | What |
| --- | --- |
| API (secret key) | `POST /v1/claims` `{account_ref, email?, label?, expires_in?}` → `{id, claim_url, ...}` · `GET /v1/claims` · `DELETE /v1/claims/{id}` (revoke) · `POST /v1/claims/redeem` `{code}` → `{account_ref, email, signup_id, reporter_token}` |
| Public | `GET /c/{token}` claim page · `POST /c/{token}` send confirmation · `GET /c/{token}/confirm/{sig}` verify and redirect |
| Dashboard | A Claims page per project: create, copy link, revoke, and a status per claim (sent, opened, email confirmed, claimed) |
| CLI | `lobby claim create --ref acct_123 [--email ...] [--label ...]`, `lobby claim list`, `lobby claim revoke <id>` |
| Webhooks | `claim.opened`, `claim.claimed` (carries `account_ref`, email, label) |
| Docs | A "Claimable accounts" guide, `/llms.txt`, and a copy-for-your-agent integration note like the feedback one |

"Opened" is recorded on the first visit to the claim page. It tells the founder a prospect looked, which is useful in a sales follow-up.

## Abuse and failure cases

- Confirmation emails from the claim page reuse the existing rate limits, disposable-domain list, MX check and suppression list. At most 5 sends per claim per day.
- Revoked, expired or already-claimed links show one plain page: "This link is no longer active. Ask {Project} for a new one."
- Redeeming a code twice returns 404, the same as an invalid invite. Redeem is one transaction: mark claimed, create or update the signup, emit the webhook.
- If the confirmed email is already a signup on that project, the claim attaches to it and marks it joined.
- Past the plan's signup limit the claim still succeeds, without a signup row (built behaviour).
- No Invite URL or Site URL set: creating a claim is refused with a message pointing at Settings.
- Plan rules follow what exists: the API and webhooks need a Launch Pack on Cloud, the dashboard works on every plan, self-hosted has no limits.

## Not in this version

- Lobby emailing the claim link for the founder. They send it themselves for now; add it when people ask, since it needs its own email template and bulk-mail checks.
- Bulk creation from CSV in the dashboard. The API and CLI cover it.
- Transferring an account that is already owned by a real user.
- Any change to how invites work.

## Testing

Table-driven Go tests in `claims_test.go` covering: create and re-create for the same `account_ref`, locked and unlocked email, expiry, revoke, code single use and 15-minute expiry, claiming with an email already on the list, the signup limit, and webhook emission. One end-to-end HTTP test walks create → open → confirm → redeem.
