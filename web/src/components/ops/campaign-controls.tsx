import { FileTextIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Link } from "wouter"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { api, campaignLabel, type CampaignStatus, type OpsCampaign } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { num } from "@/lib/format"
import { toMinutes } from "./units"

type Props = { campaign: OpsCampaign; onChange: () => Promise<void> }

// Labels for the moves the server allows. Publishing (to done) lives in ReportEditor.
const moveLabel: Partial<Record<CampaignStatus, string>> = {
  intake: "Back to intake",
  readiness: "Back to readiness",
  planning: "Start planning",
  awaiting_approval: "Send for approval",
  reporting: "Start reporting",
  cancelled: "Cancel campaign",
}
const endpoint: Partial<Record<CampaignStatus, string>> = { awaiting_approval: "send-for-approval" }

export function StatusControls({ campaign: c, moves, taskCount, onChange }: Props & { moves: CampaignStatus[]; taskCount: number }) {
  const [busy, setBusy] = useState(false)
  const [cancelling, setCancelling] = useState(false)

  async function move(to: CampaignStatus) {
    setBusy(true)
    try {
      const ep = endpoint[to]
      await api(`/ops/campaigns/${c.id}${ep ? `/${ep}` : ""}`, ep ? { body: {} } : { method: "PATCH", body: { status: to } })
      toast.success(`Moved to ${campaignLabel[to].toLowerCase()}`)
      await onChange()
    } catch (err) {
      toastError(err)
    } finally {
      setBusy(false)
    }
  }

  if (moves.length === 0) return null
  return (
    <div className="flex flex-wrap gap-2">
      {moves.filter((to) => to !== "done").map((to) =>
        to === "cancelled" ? (
          <Button key={to} variant="ghost" className="text-destructive" disabled={busy} onClick={() => setCancelling(true)}>
            {moveLabel[to]}
          </Button>
        ) : (
          <Button
            key={to}
            variant={to === "awaiting_approval" || to === "planning" || to === "reporting" ? "default" : "outline"}
            disabled={busy || (to === "awaiting_approval" && taskCount === 0)}
            onClick={() => move(to)}
          >
            {moveLabel[to] ?? campaignLabel[to]}
          </Button>
        ),
      )}
      <AlertDialog open={cancelling} onOpenChange={setCancelling}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel campaign #{c.id}?</AlertDialogTitle>
            <AlertDialogDescription>It can't be reopened. Refunds are handled in Polar, not here.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep it</AlertDialogCancel>
            <AlertDialogAction onClick={() => move("cancelled")}>Cancel campaign</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// Operator time not tied to one destination: intake, research, report writing, support.
export function CampaignMinutes({ campaign: c, taskMinutes, onChange }: Props & { taskMinutes: number }) {
  const [value, setValue] = useState(String(c.operator_minutes ?? 0))
  const minutes = toMinutes(value)
  const bad = Number.isNaN(minutes) || minutes < 0 || minutes > 100000

  async function save(e: React.FormEvent) {
    e.preventDefault()
    try {
      await api(`/ops/campaigns/${c.id}`, { method: "PATCH", body: { operator_minutes: minutes } })
      toast.success("Minutes saved")
      await onChange()
    } catch (err) {
      toastError(err)
    }
  }

  return (
    <form onSubmit={save} className="flex flex-wrap items-end gap-2">
      <div className="grid gap-2">
        <Label htmlFor="op-minutes">Campaign minutes</Label>
        <Input id="op-minutes" className="w-28" inputMode="numeric" value={value} onChange={(e) => setValue(e.target.value)} aria-invalid={bad} />
      </div>
      <Button type="submit" variant="outline" disabled={bad || minutes === (c.operator_minutes ?? 0)}>
        Save
      </Button>
      <p className="pb-2 text-xs text-muted-foreground">
        + {num(taskMinutes)} on tasks = {num((bad ? 0 : minutes) + taskMinutes)} minutes in all
      </p>
    </form>
  )
}

export function ReportEditor({ campaign: c, onChange }: Props) {
  const [note, setNote] = useState(c.report_note)
  const [busy, setBusy] = useState(false)
  const published = !!c.report_published_at

  // Publishing saves the note first: the server publishes what's stored.
  async function save(publish: boolean) {
    setBusy(true)
    try {
      if (note !== c.report_note) await api(`/ops/campaigns/${c.id}`, { method: "PATCH", body: { report_note: note } })
      if (publish) await api(`/ops/campaigns/${c.id}/publish-report`, { body: {} })
      toast.success(publish ? "Report published. The founder has been emailed." : "Report saved")
      await onChange()
    } catch (err) {
      toastError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Report</CardTitle>
        <CardDescription>
          {published
            ? "Published. The founder can read this note."
            : "The founder sees this note only after you publish. Say what was done, what's still pending, what data is missing and the next channel to try. Don't claim causes the numbers can't show."}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        <Textarea aria-label="Report note" rows={8} maxLength={10000} value={note} onChange={(e) => setNote(e.target.value)} />
        {c.approved_at && (
          <Button asChild variant="outline" className="w-fit">
            <Link href={`/ops/campaigns/${c.id}/report`}>
              <FileTextIcon />
              Open the numbers
            </Link>
          </Button>
        )}
      </CardContent>
      <CardFooter className="flex flex-wrap gap-2 border-t">
        <Button variant="outline" disabled={busy || note === c.report_note} onClick={() => save(false)}>
          Save note
        </Button>
        {c.status === "reporting" && (
          <Button disabled={busy || !note.trim()} onClick={() => save(true)}>
            Publish report
          </Button>
        )}
      </CardFooter>
    </Card>
  )
}
