import { PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { api, type Destination, type OpsCampaign, type OpsTask, type ProfileStage } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { money } from "@/lib/format"
import { useApi } from "@/lib/use-api"
import { badCents, toCents, toDollars } from "./units"

export const MAX_TASKS = 20 // maxCampaignTasks in tasks.go: a maximum, not a quota

type Props = { campaign: OpsCampaign; tasks: OpsTask[]; stage: ProfileStage | ""; onChange: () => Promise<void> }

// Adds one destination at a time to the plan. Only add what fits this product; the plan is never padded to 20.
export function PlanBuilder({ campaign: c, tasks, stage, onChange }: Props) {
  const [anyStage, setAnyStage] = useState(!stage)
  const { data } = useApi<{ data: Destination[] }>(anyStage || !stage ? "/ops/destinations" : `/ops/destinations?stage=${stage}`)
  const [pick, setPick] = useState<Destination | null>(null)
  const [reason, setReason] = useState("")
  const [owner, setOwner] = useState<"staff" | "founder">("staff")
  const [fee, setFee] = useState("")
  const [instructions, setInstructions] = useState("")
  const [busy, setBusy] = useState(false)
  const used = new Set(tasks.map((t) => t.destination_id))
  const options = (data?.data ?? []).filter((d) => d.active && !used.has(d.id))
  const full = tasks.length >= MAX_TASKS
  const cents = toCents(fee)

  function choose(id: string) {
    const d = options.find((o) => String(o.id) === id) ?? null
    setPick(d)
    if (d) {
      setOwner(d.mode === "founder" ? "founder" : "staff")
      setFee(toDollars(d.fee_cents))
    }
  }

  async function add(e: React.FormEvent) {
    e.preventDefault()
    if (!pick) return
    setBusy(true)
    try {
      await api(`/ops/campaigns/${c.id}/tasks`, { body: { destination_id: pick.id, reason, owner, fee_cents: cents, instructions } })
      toast.success(`Added ${pick.name}`)
      setPick(null)
      setReason("")
      setInstructions("")
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
        <CardTitle>
          Add to the plan <span className="ml-1 text-sm font-normal text-muted-foreground tabular-nums">{tasks.length} of up to {MAX_TASKS}</span>
        </CardTitle>
        <CardDescription>Only destinations that suit this product. Fewer good ones beat a padded list.</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="grid gap-4 sm:grid-cols-2" onSubmit={add}>
          <div className="grid gap-2 sm:col-span-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Label htmlFor="pb-dest">Destination</Label>
              {stage && (
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Checkbox checked={anyStage} onCheckedChange={(v) => setAnyStage(v === true)} />
                  Show destinations for every stage, not only {stage}
                </label>
              )}
            </div>
            <Select value={pick ? String(pick.id) : ""} onValueChange={choose} disabled={full}>
              <SelectTrigger id="pb-dest" className="w-full">
                <SelectValue placeholder={options.length ? "Choose a destination" : "No active destinations left for this filter"} />
              </SelectTrigger>
              <SelectContent>
                {options.map((d) => (
                  <SelectItem key={d.id} value={String(d.id)}>
                    {d.name} · {d.mode} · {d.fee_cents ? money(d.fee_cents) : "free"}
                    {d.stages && ` · ${d.stages.replaceAll(",", ", ")}`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {!stage && <p className="text-xs text-muted-foreground">The profile has no stage yet, so every active destination is listed.</p>}
            {pick?.rules && <p className="text-xs whitespace-pre-wrap text-muted-foreground">Rules: {pick.rules}</p>}
          </div>
          <div className="grid gap-2 sm:col-span-2">
            <Label htmlFor="pb-reason">Why it fits (the founder sees this)</Label>
            <Textarea id="pb-reason" rows={2} maxLength={1000} value={reason} onChange={(e) => setReason(e.target.value)} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pb-owner">Who does it</Label>
            <Select value={owner} onValueChange={(v) => setOwner(v as "staff" | "founder")}>
              <SelectTrigger id="pb-owner" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="staff">Staff</SelectItem>
                <SelectItem value="founder">Founder</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pb-fee">Platform fee (USD)</Label>
            <Input id="pb-fee" inputMode="decimal" value={fee} onChange={(e) => setFee(e.target.value)} aria-invalid={badCents(cents)} />
          </div>
          {owner === "founder" && (
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="pb-instr">Instructions for the founder</Label>
              <Textarea id="pb-instr" rows={3} maxLength={4000} value={instructions} onChange={(e) => setInstructions(e.target.value)} />
            </div>
          )}
          <div className="sm:col-span-2">
            <Button type="submit" disabled={busy || !pick || full || badCents(cents)}>
              <PlusIcon />
              Add destination
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
