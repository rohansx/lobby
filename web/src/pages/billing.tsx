import { ExternalLinkIcon, LoaderIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "wouter"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { PackOffer } from "@/components/pack-offer"
import { CampaignBadge } from "@/components/status-badge"
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, type Billing } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { date, money, num } from "@/lib/format"
import { useApi } from "@/lib/use-api"
import { PACK_PRICE, useBuyPack } from "@/lib/pack"
import { cn } from "@/lib/utils"

export default function BillingPage() {
  const { data, error } = useApi<Billing>("/billing")
  const [busy, setBusy] = useState(false)
  const pack = useBuyPack()

  if (error) return <LoadError error={error} />

  const portal = async () => {
    setBusy(true)
    try {
      const r = await api<{ url: string }>("/billing/portal", { body: {} })
      window.location.href = r.url
    } catch (err) {
      toastError(err)
      setBusy(false)
    }
  }

  const a = data?.account
  const packs = data?.packs ?? []

  return (
    <Page title="Plan and billing" description="Payments are handled by Polar, our merchant of record. Prices are in USD, tax included where it applies.">
      {!data ? (
        <Skeleton className="h-48" />
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              {data.plan.name} plan
              {data.subscriber && a?.status === "past_due" && <Badge variant="destructive">Payment failed</Badge>}
              {data.subscriber && a?.cancel_at_period_end && <Badge variant="outline">Ends {a.period_end ? date(a.period_end) : "soon"}</Badge>}
            </CardTitle>
            <CardDescription>
              {data.subscriber && a?.period_end
                ? a.cancel_at_period_end
                  ? `Your ${data.plan.name} subscription ends on ${date(a.period_end)}.`
                  : `Your ${data.plan.name} subscription renews ${date(a.period_end)}. It stays as it is; we no longer sell new subscriptions.`
                : data.plan.id === "pack"
                  ? "Your Launch Pack keeps these limits for good. There is nothing to renew."
                  : "Free for small lists. A Launch Pack removes the signup limit."}
              {data.subscriber && a?.status === "past_due" && " Update your card in Manage billing to keep your plan."}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-5 sm:grid-cols-3">
            <Meter label="Signups" used={data.usage.signups} limit={data.plan.signups} />
            <Meter label="Waitlists" used={data.usage.projects} limit={data.plan.projects} />
            <Meter label="Emails this month" used={data.usage.emails} limit={data.plan.emails} />
          </CardContent>
          {data.subscriber && (
            <CardFooter className="border-t">
              <Button variant="outline" disabled={busy} onClick={portal}>
                {busy ? <LoaderIcon className="animate-spin" /> : <ExternalLinkIcon />}
                Manage billing
              </Button>
              <span className="ml-3 text-xs text-muted-foreground">Update your card, get invoices or cancel.</span>
            </CardFooter>
          )}
        </Card>
      )}

      <div className="mt-10 mb-4 flex flex-wrap items-end justify-between gap-3">
        <h2 className="text-lg font-normal tracking-[-0.03em]">Launch Packs</h2>
        {data && packs.length > 0 && data.billing_enabled && (
          <Button variant="outline" disabled={pack.busy} onClick={pack.buy}>
            {pack.busy && <LoaderIcon className="animate-spin" />}
            Buy another, {PACK_PRICE}
          </Button>
        )}
      </div>
      {!data ? (
        <Skeleton className="h-24" />
      ) : packs.length === 0 ? (
        data.billing_enabled ? <PackOffer /> : <p className="text-sm text-muted-foreground">The Launch Pack opens soon. Everything on Free works today.</p>
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">Campaign</TableHead>
                <TableHead>Paid</TableHead>
                <TableHead className="text-right">Amount</TableHead>
                <TableHead className="pr-4 text-right">Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {packs.map((c) => (
                <TableRow key={c.id}>
                  <TableCell className="pl-4">
                    <Link href={`/campaigns/${c.id}`} className="font-medium hover:underline">
                      Campaign #{c.id}
                    </Link>
                  </TableCell>
                  <TableCell>{date(c.paid_at ?? c.created_at)}</TableCell>
                  <TableCell className="text-right tabular-nums">{c.price_cents > 0 ? money(c.price_cents) : "No charge"}</TableCell>
                  <TableCell className="pr-4 text-right">
                    <CampaignBadge status={c.status} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <DeleteAccount hasPassword={a?.has_password ?? true} />
    </Page>
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
