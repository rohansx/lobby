import { CheckCircle2Icon, CircleAlertIcon, ExternalLinkIcon, LoaderIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { api, type Campaign, type CampaignTask } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { money } from "@/lib/format"

export function CampaignReadiness({ campaign: c }: { campaign: Campaign }) {
  const blockers = c.readiness.filter((r) => !r.ok)
  return (
    <Card>
      <CardHeader>
        <CardTitle>Readiness review</CardTitle>
        <CardDescription>
          {c.readiness.length === 0
            ? c.status === "intake"
              ? "Submit your profile and the Lobby team reviews your landing page, demo, signup flow, assets and tracking."
              : "The review hasn't been written yet."
            : blockers.length === 0
              ? "Nothing is blocking the launch."
              : `${blockers.length} ${blockers.length === 1 ? "blocker" : "blockers"} to fix before submissions go out.`}
        </CardDescription>
      </CardHeader>
      {c.readiness.length > 0 && (
        <CardContent>
          <ul className="grid gap-3">
            {[...blockers, ...c.readiness.filter((r) => r.ok)].map((r, i) => (
              <li key={i} className="flex items-start gap-2 text-sm">
                {r.ok ? <CheckCircle2Icon className="mt-0.5 size-4 shrink-0 text-emerald-600" /> : <CircleAlertIcon className="mt-0.5 size-4 shrink-0 text-destructive" />}
                <div>
                  <div className="font-medium">
                    {r.label}
                    {!r.ok && <span className="ml-2 text-xs font-normal text-destructive">Blocker</span>}
                  </div>
                  {r.note && <p className="whitespace-pre-wrap text-muted-foreground">{r.note}</p>}
                </div>
              </li>
            ))}
          </ul>
        </CardContent>
      )}
    </Card>
  )
}

type PlanProps = { campaign: Campaign; tasks: CampaignTask[]; onChange: () => Promise<void> }

export function CampaignPlan({ campaign: c, tasks, onChange }: PlanProps) {
  const fees = tasks.reduce((n, t) => n + t.fee_cents, 0)
  if (tasks.length === 0) {
    return <p className="text-sm text-muted-foreground">{c.status === "planning" ? "The Lobby team is building your plan." : "There's no plan yet. It comes after the readiness review."}</p>
  }
  return (
    <div className="grid gap-6">
      {c.changes_note && c.status === "planning" && (
        <div className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
          <span className="font-medium">Changes you asked for:</span> <span className="whitespace-pre-wrap">{c.changes_note}</span>
        </div>
      )}
      <Card>
        <CardHeader>
          <CardTitle>Launch plan</CardTitle>
          <CardDescription>
            {tasks.length} of up to 20 destinations, chosen for your product and stage. Each platform decides whether and when it publishes; acceptance isn't guaranteed.
          </CardDescription>
        </CardHeader>
        <CardContent className="px-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-6">Destination</TableHead>
                <TableHead>Why it fits</TableHead>
                <TableHead>Who does it</TableHead>
                <TableHead className="pr-6 text-right">Platform fee</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tasks.map((t) => (
                <TableRow key={t.id}>
                  <TableCell className="pl-6 align-top">
                    <a href={t.destination_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium hover:underline">
                      {t.destination_name}
                      <ExternalLinkIcon className="size-3 text-muted-foreground" />
                    </a>
                  </TableCell>
                  <TableCell className="max-w-md align-top whitespace-pre-wrap text-muted-foreground">{t.reason || "—"}</TableCell>
                  <TableCell className="align-top">{t.owner === "founder" ? <Badge variant="outline">You</Badge> : <Badge variant="secondary">Lobby team</Badge>}</TableCell>
                  <TableCell className="pr-6 text-right align-top tabular-nums">{t.fee_cents > 0 ? money(t.fee_cents) : "Free"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell colSpan={3} className="pl-6">
                  Platform fees, total
                </TableCell>
                <TableCell className="pr-6 text-right tabular-nums">{money(fees)}</TableCell>
              </TableRow>
            </TableFooter>
          </Table>
          <p className="mt-3 px-6 text-xs text-muted-foreground">
            Platform fees are charged by each destination and are separate from the Launch Pack{c.price_cents > 0 && `, which you paid ${money(c.price_cents)} for`}. If a destination turns out to be unavailable, its task records why.
          </p>
        </CardContent>
      </Card>
      {c.status === "awaiting_approval" && <ApprovePlan campaign={c} onChange={onChange} />}
    </div>
  )
}

function ApprovePlan({ campaign: c, onChange }: { campaign: Campaign; onChange: () => Promise<void> }) {
  const [busy, setBusy] = useState("")
  const [asking, setAsking] = useState(false)
  const [note, setNote] = useState("")
  const move = async (action: "approve" | "request-changes") => {
    setBusy(action)
    try {
      await api(`/campaigns/${c.id}/${action}`, { body: action === "approve" ? {} : { note } })
      toast.success(action === "approve" ? "Plan approved. Work starts now." : "Sent back with your note.")
      await onChange()
    } catch (err) {
      toastError(err)
      setBusy("")
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Approve the plan</CardTitle>
        <CardDescription>Approving also approves the latest saved version of your product profile; every submission is written from it.</CardDescription>
      </CardHeader>
      {asking && (
        <CardContent className="grid gap-2">
          <Label htmlFor="changes-note">What should change?</Label>
          <Textarea id="changes-note" rows={3} maxLength={2000} value={note} onChange={(e) => setNote(e.target.value)} />
        </CardContent>
      )}
      <CardFooter className="flex flex-wrap gap-2 border-t">
        {!asking ? (
          <>
            <Button disabled={!!busy} onClick={() => move("approve")}>
              {busy === "approve" && <LoaderIcon className="animate-spin" />}
              Approve plan
            </Button>
            <Button variant="outline" disabled={!!busy} onClick={() => setAsking(true)}>
              Request changes
            </Button>
          </>
        ) : (
          <>
            <Button disabled={!!busy || !note.trim()} onClick={() => move("request-changes")}>
              {busy === "request-changes" && <LoaderIcon className="animate-spin" />}
              Send back
            </Button>
            <Button variant="ghost" disabled={!!busy} onClick={() => setAsking(false)}>
              Cancel
            </Button>
          </>
        )}
      </CardFooter>
    </Card>
  )
}
