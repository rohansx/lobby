import { useState } from "react"
import { toast } from "sonner"
import { Page } from "@/components/page"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, type OpsAccount, type OpsData } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { ago, dateTime, num } from "@/lib/format"
import { useSession } from "@/lib/session"
import { useApi } from "@/lib/use-api"

// Operator page for whoever runs a Lobby Cloud instance (LOBBY_OPERATORS).
export default function Ops() {
  const { me } = useSession()
  const { data, reload } = useApi<OpsData>("/ops")
  const [filter, setFilter] = useState("")
  const [suspending, setSuspending] = useState<OpsAccount | null>(null)

  async function act(ac: OpsAccount, action: "suspend" | "unsuspend") {
    try {
      await api(`/ops/accounts/${ac.id}/${action}`, { method: "POST" })
      toast.success(action === "suspend" ? `Suspended ${ac.email}` : `Unsuspended ${ac.email}`)
      reload()
    } catch (err) {
      toastError(err)
    }
  }

  const t = data?.totals
  const tiles: [string, string, string?][] = t
    ? [
        ["Accounts", num(t.accounts), `${num(t.verified)} verified`],
        ["Paying", num(Object.values(t.paying).reduce((a, b) => a + b, 0)), Object.entries(t.paying).map(([p, n]) => `${n} ${p}`).join(", ") || "none yet"],
        ["MRR (est.)", `$${num(t.mrr)}`, "yearly plans at their monthly rate"],
        ["Signups", num(t.signups), `${num(t.signups_7d)} in the last 7 days`],
        ["Emails this month", num(t.emails_month)],
        ["Email queue", num(t.queued_jobs), `${num(t.failed_jobs)} failed`],
      ]
    : []
  const q = filter.trim().toLowerCase()
  const rows = data?.accounts.filter((a) => !q || a.email.toLowerCase().includes(q)) ?? []

  return (
    <Page title="Operator" description="Every account on this Lobby Cloud instance.">
      {!data ? (
        <Skeleton className="h-40" />
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {tiles.map(([label, value, hint]) => (
              <Card key={label} className="gap-1 py-4">
                <CardHeader className="px-4">
                  <CardDescription>{label}</CardDescription>
                  <CardTitle className="text-2xl tabular-nums">{value}</CardTitle>
                </CardHeader>
                {hint && <CardContent className="px-4 text-xs text-muted-foreground">{hint}</CardContent>}
              </Card>
            ))}
          </div>

          <div className="mt-8 mb-3 flex flex-wrap items-center gap-3">
            <h2 className="flex-1 text-sm font-medium">
              Accounts
              {data.totals.truncated && <span className="ml-2 font-normal text-muted-foreground">newest {num(data.totals.accounts_shown)} of {num(data.totals.accounts)}</span>}
            </h2>
            <Input className="max-w-64" placeholder="Filter by email" value={filter} onChange={(e) => setFilter(e.target.value)} aria-label="Filter by email" />
          </div>
          <Card className="overflow-hidden py-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-4">Email</TableHead>
                  <TableHead>Plan</TableHead>
                  <TableHead className="text-right">Waitlists</TableHead>
                  <TableHead className="text-right">Signups</TableHead>
                  <TableHead className="text-right">Emails</TableHead>
                  <TableHead>Joined</TableHead>
                  <TableHead className="w-28" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={7} className="py-10 text-center text-muted-foreground">
                      No accounts match.
                    </TableCell>
                  </TableRow>
                )}
                {rows.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell className="max-w-72 pl-4">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate">{a.email}</span>
                        {!a.verified && <Badge variant="outline">unverified</Badge>}
                        {a.suspended && <Badge variant="destructive">suspended</Badge>}
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="capitalize">{a.plan}</span>
                      {a.interval && <span className="text-muted-foreground"> / {a.interval}</span>}
                      {a.status && a.status !== "active" && <span className="ml-1 text-xs text-muted-foreground">({a.status})</span>}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{num(a.projects)}</TableCell>
                    <TableCell className="text-right tabular-nums">{num(a.signups)}</TableCell>
                    <TableCell className="text-right tabular-nums">{num(a.emails_month)}</TableCell>
                    <TableCell className="text-muted-foreground" title={dateTime(a.created_at)}>
                      {ago(a.created_at)}
                    </TableCell>
                    <TableCell className="pr-4 text-right">
                      {a.suspended ? (
                        <Button variant="outline" size="sm" onClick={() => act(a, "unsuspend")}>
                          Unsuspend
                        </Button>
                      ) : (
                        a.email !== me.email && (
                          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setSuspending(a)}>
                            Suspend
                          </Button>
                        )
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>
        </>
      )}

      <AlertDialog open={!!suspending} onOpenChange={(o) => !o && setSuspending(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Suspend {suspending?.email}?</AlertDialogTitle>
            <AlertDialogDescription>
              Their waitlists stop taking signups and nothing is emailed from the account, including queued mail. They can still sign in. You can undo this.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => suspending && act(suspending, "suspend")}>Suspend</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Page>
  )
}
