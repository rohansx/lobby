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
  const [error, setError] = useState("")
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
