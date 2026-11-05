import { ExternalLinkIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { CopyField } from "@/components/copy-field"
import { TaskBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { api, type OpsCampaign, type OpsTask, type TaskState } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { badCents, dayInput, fromDayInput, toCents, toDollars, toMinutes } from "./units"

const states: TaskState[] = ["planned", "needs_assets", "awaiting_approval", "awaiting_founder", "ready", "submitted", "live", "rejected", "blocked"]

type Form = { owner: "staff" | "founder"; state: TaskState; reason: string; fee: string; instructions: string; submitted: string; live: string; listing_url: string; evidence: string; exception_note: string; minutes: string }

const toForm = (t: OpsTask): Form => ({
  owner: t.owner,
  state: t.state,
  reason: t.reason,
  fee: toDollars(t.fee_cents),
  instructions: t.instructions,
  submitted: dayInput(t.submitted_at),
  live: dayInput(t.live_at),
  listing_url: t.listing_url,
  evidence: t.evidence,
  exception_note: t.exception_note,
  minutes: String(t.minutes ?? 0),
})

// One destination on the plan, with everything an operator records about it.
export function TaskCard({ campaign: c, task: t, onChange }: { campaign: OpsCampaign; task: OpsTask; onChange: () => Promise<void> }) {
  const [form, setForm] = useState(() => toForm(t))
  const [busy, setBusy] = useState(false)
  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => ({ ...f, [k]: v }))
  const cents = toCents(form.fee)
  const minutes = toMinutes(form.minutes)
  const bad = badCents(cents) || Number.isNaN(minutes) || minutes < 0 || minutes > 100000
  const dirty = JSON.stringify(form) !== JSON.stringify(toForm(t))
  const planning = c.status === "planning"
  const link = t.campaign_link

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      const { submitted, live, fee, ...rest } = form
      // Dates go only when changed (undefined is left out of the JSON). 0 clears one; the server
      // stamps today when the state is submitted or live and the date is empty.
      const day = (v: string, was: number | null) => (v === dayInput(was) ? undefined : (fromDayInput(v) ?? 0))
      await api(`/ops/campaigns/${c.id}/tasks/${t.id}`, {
        method: "PATCH",
        body: { ...rest, fee_cents: toCents(fee), minutes, submitted_at: day(submitted, t.submitted_at), live_at: day(live, t.live_at) },
      })
      toast.success(`Saved ${t.destination_name}`)
      await onChange()
    } catch (err) {
      toastError(err)
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    try {
      await api(`/ops/campaigns/${c.id}/tasks/${t.id}`, { method: "DELETE" })
      await onChange()
    } catch (err) {
      toastError(err)
    }
  }

  const text = (k: "listing_url" | "evidence" | "exception_note" | "reason" | "instructions", label: string, max: number, area = true) => (
    <div className={area ? "grid gap-2 sm:col-span-2" : "grid gap-2"}>
      <Label htmlFor={`t${t.id}-${k}`}>{label}</Label>
      {area ? (
        <Textarea id={`t${t.id}-${k}`} rows={2} maxLength={max} value={form[k]} onChange={(e) => set(k, e.target.value)} />
      ) : (
        <Input id={`t${t.id}-${k}`} type="url" placeholder="https://" maxLength={max} value={form[k]} onChange={(e) => set(k, e.target.value)} />
      )}
    </div>
  )
  const select = <K extends "owner" | "state">(k: K, label: string, values: Form[K][]) => (
    <div className="grid gap-2">
      <Label htmlFor={`t${t.id}-${k}`}>{label}</Label>
      <Select value={form[k]} onValueChange={(v) => set(k, v as Form[K])}>
        <SelectTrigger id={`t${t.id}-${k}`} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {values.map((v) => (
            <SelectItem key={v} value={v}>
              {v.replaceAll("_", " ")}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
  const field = (k: "submitted" | "live" | "fee" | "minutes", label: string, type: string) => (
    <div className="grid gap-2">
      <Label htmlFor={`t${t.id}-${k}`}>{label}</Label>
      <Input id={`t${t.id}-${k}`} type={type} inputMode={type === "text" ? "decimal" : undefined} value={form[k]} onChange={(e) => set(k, e.target.value)} />
    </div>
  )

  return (
    <form onSubmit={save}>
      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            <a href={t.destination_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:underline">
              {t.destination_name}
              <ExternalLinkIcon className="size-3 text-muted-foreground" />
            </a>
            <TaskBadge state={t.state} ops />
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-4">
          {select("state", "State", states)}
          {select("owner", "Who does it", ["staff", "founder"])}
          {field("fee", "Platform fee (USD)", "text")}
          {field("minutes", "Minutes spent", "number")}
          <div className="grid gap-4 sm:col-span-4 sm:grid-cols-2">
            {text("reason", "Why it fits (the founder sees this)", 1000)}
            {form.owner === "founder" && text("instructions", "Instructions for the founder", 4000)}
          </div>
          {!planning && (
            <>
              {field("submitted", "Submitted on", "date")}
              {field("live", "Live on", "date")}
              <div className="sm:col-span-2">{text("listing_url", "Listing URL", 500, false)}</div>
              <div className="grid gap-4 sm:col-span-4 sm:grid-cols-2">
                {text("evidence", "Evidence (confirmation, screenshot reference)", 4000)}
                {text("exception_note", "Exception (why rejected, blocked or unavailable)", 2000)}
              </div>
            </>
          )}
          {link && (
            <div className="grid gap-2 sm:col-span-4">
              <Label>Campaign link</Label>
              <CopyField value={link} />
            </div>
          )}
        </CardContent>
        <CardFooter className="flex flex-wrap gap-2 border-t">
          <Button type="submit" disabled={busy || !dirty || bad}>
            Save
          </Button>
          {planning && (
            <Button type="button" variant="ghost" className="text-destructive" onClick={remove}>
              <Trash2Icon />
              Remove from plan
            </Button>
          )}
        </CardFooter>
      </Card>
    </form>
  )
}
