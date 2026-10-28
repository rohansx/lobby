import { useState } from "react"
import { useLocation } from "wouter"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, type Me } from "@/lib/api"

export function AuthPage({ mode, me, onDone }: { mode: "setup" | "login"; me: Me; onDone: () => Promise<void> }) {
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const [, navigate] = useLocation()
  const setup = mode === "setup"

  return (
    <div className="flex min-h-svh items-center justify-center bg-muted/40 p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center justify-center gap-2.5 text-lg font-medium tracking-[-0.03em]">
          <img src="/favicon.svg" alt="" className="size-8 rounded-lg dark:ring-1 dark:ring-white/15" />
          Lobby
        </div>
        <Card>
          <CardHeader>
            <CardTitle>{setup ? "Create the admin account" : "Sign in"}</CardTitle>
            <CardDescription>{setup ? "This page disappears once the account exists." : "Welcome back."}</CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="grid gap-4"
              onSubmit={async (e) => {
                e.preventDefault()
                setBusy(true)
                setError("")
                try {
                  await api(setup ? "/setup" : "/login", { body: { email, password } })
                  await onDone()
                  if (setup) navigate("/")
                } catch (err) {
                  setError((err as Error).message)
                } finally {
                  setBusy(false)
                }
              }}
            >
              <div className="grid gap-2">
                <Label htmlFor="email">Email</Label>
                <Input id="email" type="email" autoComplete="username" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete={setup ? "new-password" : "current-password"}
                  required
                  minLength={setup ? 10 : undefined}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                {setup && <p className="text-xs text-muted-foreground">At least 10 characters.</p>}
              </div>
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" disabled={busy} className="w-full">
                {setup ? "Create account" : "Sign in"}
              </Button>
            </form>
          </CardContent>
        </Card>
        {setup && (
          <p className="mt-4 text-center text-xs text-muted-foreground">
            Email goes out via <span className="font-medium text-foreground">{me.mailer}</span>
            {me.from ? <> from {me.from}</> : <>. Set LOBBY_FROM_EMAIL and RESEND_API_KEY or SMTP_URL to send for real.</>}
          </p>
        )}
      </div>
    </div>
  )
}
