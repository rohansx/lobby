import { CheckIcon, ExternalLinkIcon, LoaderIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { toast } from "sonner"
import { useLocation } from "wouter"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { api, type Billing, type Plan } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { date, num } from "@/lib/format"
import { useSession } from "@/lib/session"
import { useApi } from "@/lib/use-api"
import { cn } from "@/lib/utils"

const features: Record<string, string[]> = {
  free: ["500 signups", "1 waitlist", "1,000 emails a month", "Referrals and invite waves", "Lobby badge on hosted pages"],
  launch: ["Unlimited signups", "3 waitlists", "5,000 emails a month", "Webhooks and REST API", "No Lobby badge"],
  pro: ["Unlimited signups", "Unlimited waitlists", "25,000 emails a month", "Webhooks and REST API", "Priority support"],
}

export default function BillingPage() {
  const { data, error, reload } = useApi<Billing>("/billing")
  const { refreshMe } = useSession()
  const [yearly, setYearly] = useState(true)
  const [busy, setBusy] = useState("")
  const [activating, setActivating] = useState(() => new URLSearchParams(window.location.search).has("checkout"))
  const [, navigate] = useLocation()

  // Back from checkout: the plan changes when Polar's webhook lands, usually
  // within seconds, so poll briefly.
  useEffect(() => {
    if (!activating) return
    let tries = 0
    const t = setInterval(async () => {
      tries++
      const b = await api<Billing>("/billing").catch(() => null)
      if (b && b.plan.id !== "free") {
        clearInterval(t)
        setActivating(false)
        navigate("/billing", { replace: true })
        await Promise.all([reload(), refreshMe()])
        toast.success(`You're on ${b.plan.name}. Thanks for supporting Lobby.`)
      } else if (tries >= 15) {
        clearInterval(t)
        setActivating(false)
        navigate("/billing", { replace: true })
        toast("Payment received. Your plan will update in a minute or two.")
      }
    }, 2000)
    return () => clearInterval(t)
  }, [activating, navigate, reload, refreshMe])

  if (error) return <LoadError error={error} />

  const go = async (key: string, path: string, body?: object) => {
    setBusy(key)
    try {
      const r = await api<{ url: string }>(path, { body: body ?? {} })
      window.location.href = r.url
    } catch (err) {
      toastError(err)
      setBusy("")
    }
  }

  const a = data?.account
  const paid = data && data.plan.id !== "free"

  return (
    <Page title="Plan and billing" description="Payments are handled by Polar, our merchant of record. Prices are in USD, tax included where it applies.">
      {activating && (
        <div className="mb-6 flex items-center gap-2 rounded-lg border bg-muted/50 px-4 py-3 text-sm">
          <LoaderIcon className="size-4 animate-spin" />
          Payment received. Activating your plan…
        </div>
      )}

      {!data ? (
        <Skeleton className="h-48" />
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              {data.plan.name} plan
              {a?.status === "past_due" && <Badge variant="destructive">Payment failed</Badge>}
              {a?.cancel_at_period_end && <Badge variant="outline">Ends {a.period_end ? date(a.period_end) : "soon"}</Badge>}
            </CardTitle>
            <CardDescription>
              {paid && a?.period_end
                ? a.cancel_at_period_end
                  ? `You keep ${data.plan.name} until ${date(a.period_end)}, then move to Free.`
                  : `Renews ${date(a.period_end)}, ${a.interval === "year" ? `$${data.plan.yearly * 12} a year` : `$${data.plan.monthly} a month`}.`
                : "Free forever for small lists. Upgrade when your launch takes off."}
              {a?.status === "past_due" && " Update your card in Manage billing to keep your plan."}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-5 sm:grid-cols-3">
            <Meter label="Signups" used={data.usage.signups} limit={data.plan.signups} />
            <Meter label="Waitlists" used={data.usage.projects} limit={data.plan.projects} />
            <Meter label="Emails this month" used={data.usage.emails} limit={data.plan.emails} />
          </CardContent>
          {paid && (
            <CardFooter className="border-t">
              <Button variant="outline" disabled={!!busy} onClick={() => go("portal", "/billing/portal")}>
                {busy === "portal" ? <LoaderIcon className="animate-spin" /> : <ExternalLinkIcon />}
                Manage billing
              </Button>
              <span className="ml-3 text-xs text-muted-foreground">Change plan, update your card, get invoices or cancel.</span>
            </CardFooter>
          )}
        </Card>
      )}

      <div className="mt-10 mb-4 flex flex-wrap items-end justify-between gap-3">
        <h2 className="text-lg font-normal tracking-[-0.03em]">Plans</h2>
        <div className="inline-flex rounded-lg border p-0.5 text-sm" role="group" aria-label="Billing period">
          {[false, true].map((y) => (
            <button
              key={String(y)}
              type="button"
              aria-pressed={yearly === y}
              onClick={() => setYearly(y)}
              className={cn("rounded-md px-3 py-1 transition-colors", yearly === y ? "bg-foreground text-background" : "text-muted-foreground hover:text-foreground")}
            >
              {y ? "Yearly · save 20%" : "Monthly"}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        {(data?.plans ?? []).map((p) => {
          const current = data?.plan.id === p.id
          return (
            <Card key={p.id} className={cn(p.id === "launch" && "border-foreground/30")}>
              <CardHeader>
                <CardTitle className="flex items-center justify-between">
                  {p.name}
                  {current ? <Badge variant="secondary">Current</Badge> : p.id === "launch" ? <Badge variant="outline">Most popular</Badge> : null}
                </CardTitle>
                <div className="pt-2">
                  <span className="text-4xl font-light tabular-nums tracking-[-0.04em]">${yearly ? p.yearly : p.monthly}</span>
                  <span className="text-sm text-muted-foreground">{p.id === "free" ? " forever" : " / month"}</span>
                  {p.id !== "free" && yearly && <div className="text-xs text-muted-foreground">${p.yearly * 12} billed yearly</div>}
                  {p.id !== "free" && !yearly && <div className="text-xs text-muted-foreground">billed monthly</div>}
                  {p.id === "free" && <div className="text-xs text-muted-foreground">no card needed</div>}
                </div>
              </CardHeader>
              <CardContent>
                <ul className="grid gap-2 text-sm">
                  {features[p.id]?.map((f) => (
                    <li key={f} className="flex items-start gap-2">
                      <CheckIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                      {f}
                    </li>
                  ))}
                </ul>
              </CardContent>
              <CardFooter className="mt-auto">
                <PlanButton plan={p} current={current} paid={!!paid} enabled={!!data?.billing_enabled} busy={busy} verified={!!a?.verified} onCheckout={() => go(p.id, "/billing/checkout", { plan: p.id, interval: yearly ? "year" : "month" })} onPortal={() => go("portal", "/billing/portal")} />
              </CardFooter>
            </Card>
          )
        })}
      </div>
      {data && !data.billing_enabled && <p className="mt-4 text-sm text-muted-foreground">Paid plans open soon. Everything on Free works today.</p>}

      <DeleteAccount hasPassword={a?.has_password ?? true} />
    </Page>
  )
}

function PlanButton(props: { plan: Plan; current: boolean; paid: boolean; enabled: boolean; busy: string; verified: boolean; onCheckout: () => void; onPortal: () => void }) {
  const { plan, current, paid, enabled, busy, verified } = props
  if (current) return <Button variant="outline" className="w-full" disabled>Your plan</Button>
  if (plan.id === "free") {
    return paid ? (
      <Button variant="outline" className="w-full" disabled={!!busy} onClick={props.onPortal}>
        Downgrade
      </Button>
    ) : null
  }
  if (paid) {
    return (
      <Button variant="outline" className="w-full" disabled={!!busy} onClick={props.onPortal}>
        Switch to {plan.name}
      </Button>
    )
  }
  return (
    <Button className="w-full" disabled={!enabled || !verified || !!busy} onClick={props.onCheckout} title={!verified ? "Confirm your email first" : undefined}>
      {busy === plan.id && <LoaderIcon className="animate-spin" />}
      Upgrade to {plan.name}
    </Button>
  )
}

function Meter({ label, used, limit }: { label: string; used: number; limit: number }) {
  const unlimited = limit < 0
  const ratio = unlimited ? 0 : Math.min(1, used / Math.max(limit, 1))
  return (
    <div>
      <div className="flex items-baseline justify-between text-sm">
        <span className="text-muted-foreground">{label}</span>
        <span className="tabular-nums">
          {num(used)}
          <span className="text-muted-foreground"> / {unlimited ? "unlimited" : num(limit)}</span>
        </span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted" role="meter" aria-label={label} aria-valuenow={used} aria-valuemin={0} aria-valuemax={unlimited ? undefined : limit}>
        <div className={cn("h-full rounded-full transition-[width]", ratio >= 0.8 ? "bg-amber-500" : "bg-foreground/70")} style={{ width: `${unlimited ? 0 : Math.max(ratio * 100, used > 0 ? 2 : 0)}%` }} />
      </div>
    </div>
  )
}

function DeleteAccount({ hasPassword }: { hasPassword: boolean }) {
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  return (
    <div className="mt-12 flex flex-wrap items-center justify-between gap-3 border-t pt-6">
      <div>
        <div className="text-sm font-medium">Delete account</div>
        <p className="text-sm text-muted-foreground">Removes your account, every waitlist and every signup. This can't be undone.</p>
      </div>
      <AlertDialog onOpenChange={() => setPassword("")}>
        <AlertDialogTrigger asChild>
          <Button variant="outline" className="text-destructive">
            Delete account
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <form
            className="grid gap-4"
            onSubmit={async (e) => {
              e.preventDefault()
              setBusy(true)
              try {
                await api("/account", { method: "DELETE", body: { password } })
                window.location.href = "/signup"
              } catch (err) {
                toastError(err)
                setBusy(false)
              }
            }}
          >
            <AlertDialogHeader>
              <AlertDialogTitle>Delete your account?</AlertDialogTitle>
              <AlertDialogDescription>Export anything you need first. {hasPassword ? "Enter your password" : "Type your email address"} to confirm.</AlertDialogDescription>
            </AlertDialogHeader>
            {hasPassword ? (
              <Input type="password" autoComplete="current-password" required placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} />
            ) : (
              <Input type="email" required placeholder="you@example.com" value={password} onChange={(e) => setPassword(e.target.value)} />
            )}
            <AlertDialogFooter>
              <AlertDialogCancel type="button">Cancel</AlertDialogCancel>
              <Button type="submit" variant="destructive" disabled={busy || !password}>
                Delete everything
              </Button>
            </AlertDialogFooter>
          </form>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
