import { useState, type ReactNode } from "react"
import { Link, useLocation } from "wouter"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, type Me } from "@/lib/api"

export type AuthMode = "setup" | "login" | "signup" | "forgot" | "reset"

const copy: Record<AuthMode, { title: string; description: string; button: string }> = {
  setup: { title: "Create the admin account", description: "This page disappears once the account exists.", button: "Create account" },
  login: { title: "Sign in", description: "Welcome back.", button: "Sign in" },
  signup: { title: "Create your account", description: "Free for your first 500 signups. No card needed.", button: "Create account" },
  forgot: { title: "Reset your password", description: "We'll email you a link to choose a new one.", button: "Send reset link" },
  reset: { title: "Choose a new password", description: "You'll be signed in straight after.", button: "Save password" },
}

export function AuthPage({ mode, me, onDone }: { mode: AuthMode; me: Me; onDone: () => Promise<void> }) {
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  // OAuth failures come back as /login?error=...
  const [error, setError] = useState(() => new URLSearchParams(window.location.search).get("error") ?? "")
  const [sent, setSent] = useState(false)
  const [busy, setBusy] = useState(false)
  const [, navigate] = useLocation()
  const c = copy[mode]
  const newPassword = mode === "setup" || mode === "signup" || mode === "reset"

  async function submit() {
    const token = new URLSearchParams(window.location.search).get("token") ?? ""
    switch (mode) {
      case "forgot":
        await api("/forgot", { body: { email } })
        setSent(true)
        return
      case "reset":
        await api("/reset", { body: { token, password } })
        break
      default:
        await api(`/${mode}`, { body: { email, password } })
    }
    await onDone()
    // Signups from a paid plan on the pricing page land on billing.
    const plan = new URLSearchParams(window.location.search).get("plan")
    if (mode === "signup" && plan) navigate("/billing")
    else if (mode !== "login") navigate("/dashboard")
  }

  return (
    <AuthFrame>
      <Card>
        <CardHeader>
          <CardTitle>{c.title}</CardTitle>
          <CardDescription>{c.description}</CardDescription>
        </CardHeader>
        <CardContent>
          {(mode === "login" || mode === "signup") && me.providers?.length > 0 && <OAuthButtons providers={me.providers} />}
          {sent ? (
            <p className="text-sm text-muted-foreground">
              If there's an account for <span className="text-foreground">{email}</span>, a reset link is on its way. It works for one hour.
            </p>
          ) : (
            <form
              className="grid gap-4"
              onSubmit={async (e) => {
                e.preventDefault()
                setBusy(true)
                setError("")
                try {
                  await submit()
                } catch (err) {
                  setError((err as Error).message)
                } finally {
                  setBusy(false)
                }
              }}
            >
              {mode !== "reset" && (
                <div className="grid gap-2">
                  <Label htmlFor="email">Email</Label>
                  <Input id="email" type="email" autoComplete="username" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
                </div>
              )}
              {mode !== "forgot" && (
                <div className="grid gap-2">
                  <div className="flex items-baseline justify-between">
                    <Label htmlFor="password">{mode === "reset" ? "New password" : "Password"}</Label>
                    {mode === "login" && me.cloud && (
                      <Link href="/forgot" className="text-xs text-muted-foreground hover:text-foreground">
                        Forgot it?
                      </Link>
                    )}
                  </div>
                  <Input
                    id="password"
                    type="password"
                    autoComplete={newPassword ? "new-password" : "current-password"}
                    required
                    autoFocus={mode === "reset"}
                    minLength={newPassword ? 10 : undefined}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                  {newPassword && <p className="text-xs text-muted-foreground">At least 10 characters.</p>}
                </div>
              )}
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" disabled={busy} className="w-full">
                {c.button}
              </Button>
              {mode === "signup" && (
                <p className="text-center text-xs text-muted-foreground">
                  By signing up you agree to the{" "}
                  <a className="underline underline-offset-2" href="https://trylobby.app/terms" target="_blank" rel="noreferrer">
                    terms
                  </a>{" "}
                  and{" "}
                  <a className="underline underline-offset-2" href="https://trylobby.app/privacy" target="_blank" rel="noreferrer">
                    privacy policy
                  </a>
                  .
                </p>
              )}
            </form>
          )}
        </CardContent>
      </Card>
      {me.cloud && (
        <p className="mt-4 text-center text-sm text-muted-foreground">
          {mode === "signup" ? (
            <>
              Have an account? <SwitchLink href="/login">Sign in</SwitchLink>
            </>
          ) : mode === "login" ? (
            <>
              New to Lobby? <SwitchLink href="/signup">Create an account</SwitchLink>
            </>
          ) : (
            <SwitchLink href="/login">Back to sign in</SwitchLink>
          )}
        </p>
      )}
      {mode === "setup" && (
        <p className="mt-4 text-center text-xs text-muted-foreground">
          Email goes out via <span className="font-medium text-foreground">{me.mailer}</span>
          {me.from ? <> from {me.from}</> : <>. Set LOBBY_FROM_EMAIL and RESEND_API_KEY or SMTP_URL to send for real.</>}
        </p>
      )}
    </AuthFrame>
  )
}

const providerLabels = { google: "Google", github: "GitHub" }

// OAuthButtons start a full-page redirect to the provider; the server sends
// the person back signed in, to ?next= or the dashboard.
function OAuthButtons({ providers }: { providers: Me["providers"] }) {
  const q = new URLSearchParams(window.location.search)
  const next = q.get("plan") ? "/billing" : (q.get("next") ?? "/dashboard")
  return (
    <div className="mb-5 grid gap-2.5">
      {providers.map((p) => (
        <Button key={p} variant="outline" className="w-full" asChild>
          <a href={`/auth/${p}/start?next=${encodeURIComponent(next)}`}>
            {p === "google" ? <GoogleIcon /> : <GitHubIcon />}
            Continue with {providerLabels[p]}
          </a>
        </Button>
      ))}
      <div className="mt-2.5 flex items-center gap-3 text-xs text-muted-foreground">
        <span className="h-px flex-1 bg-border" />
        or with email
        <span className="h-px flex-1 bg-border" />
      </div>
    </div>
  )
}

function GoogleIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.76h3.56c2.08-1.92 3.28-4.74 3.28-8.09Z" />
      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.56-2.76c-.98.66-2.23 1.06-3.72 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84A11 11 0 0 0 12 23Z" />
      <path fill="#FBBC05" d="M5.84 14.11A6.6 6.6 0 0 1 5.5 12c0-.73.13-1.44.34-2.11V7.05H2.18A11 11 0 0 0 1 12c0 1.77.42 3.45 1.18 4.95l3.66-2.84Z" />
      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1A11 11 0 0 0 2.18 7.05l3.66 2.84C6.71 7.31 9.14 5.38 12 5.38Z" />
    </svg>
  )
}

function GitHubIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z" />
    </svg>
  )
}

function SwitchLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link href={href} className="font-medium text-foreground underline-offset-4 hover:underline">
      {children}
    </Link>
  )
}

function AuthFrame({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh items-center justify-center bg-muted/40 p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center justify-center gap-2.5 text-lg font-medium tracking-[-0.03em]">
          <img src="/favicon.svg" alt="" className="size-8 rounded-lg dark:ring-1 dark:ring-white/15" />
          Lobby
        </div>
        {children}
      </div>
    </div>
  )
}
