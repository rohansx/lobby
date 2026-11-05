import { useState, type ReactNode } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { api, PROFILE_STAGES, type Destination } from "@/lib/api"
import { badCents, dayInput, fromDayInput, toCents, toDollars } from "./units"

type Form = Omit<Destination, "id" | "fee_cents" | "created_at" | "updated_at"> & { fee: string }

const blank: Form = { slug: "", name: "", url: "", audience: "", stages: "", fee: "0", fee_note: "", fields: "", rules: "", mode: "staff", active: true, last_verified_at: null, notes: "" }

const modes: { value: Destination["mode"]; label: string }[] = [
  { value: "staff", label: "Staff submit it" },
  { value: "founder", label: "Founder submits it (their own account)" },
  { value: "guided", label: "Guided: we prepare, founder completes" },
]

// Creates a destination, or edits one when `destination` is set. Every catalogue field is here.
export function DestinationDialog({ destination, children, onSaved }: { destination?: Destination; children: ReactNode; onSaved: () => void }) {
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<Form>(blank)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => ({ ...f, [k]: v }))
  const stages = form.stages.split(",").filter(Boolean)
  const cents = toCents(form.fee)

  function openChange(o: boolean) {
    setOpen(o)
    setError("")
    if (o) setForm(destination ? { ...destination, fee: toDollars(destination.fee_cents) } : blank)
  }

  function toggleStage(s: string, on: boolean) {
    set("stages", PROFILE_STAGES.filter((x) => (x === s ? on : stages.includes(x))).join(","))
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError("")
    try {
      // fee: undefined drops the dollars field from the JSON body.
      await api(destination ? `/ops/destinations/${destination.id}` : "/ops/destinations", { method: destination ? "PATCH" : "POST", body: { ...form, fee: undefined, fee_cents: cents } })
      toast.success(destination ? `Saved ${form.name}` : `Added ${form.name}`)
      setOpen(false)
      onSaved()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const text = (k: "slug" | "name" | "url" | "fee_note", label: string, max: number, props: React.ComponentProps<typeof Input> = {}) => (
    <div className="grid gap-2">
      <Label htmlFor={`d-${k}`}>{label}</Label>
      <Input id={`d-${k}`} maxLength={max} value={form[k]} onChange={(e) => set(k, e.target.value)} {...props} />
    </div>
  )
  const area = (k: "audience" | "fields" | "rules" | "notes", label: string, max: number, hint?: string) => (
    <div className="grid gap-2 sm:col-span-2">
      <Label htmlFor={`d-${k}`}>{label}</Label>
      <Textarea id={`d-${k}`} rows={3} maxLength={max} value={form[k]} onChange={(e) => set(k, e.target.value)} />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )

  return (
    <Dialog open={open} onOpenChange={openChange}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <form className="grid gap-4" onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{destination ? `Edit ${destination.name}` : "New destination"}</DialogTitle>
            <DialogDescription>Founders only see the name, link, fee and the reason and instructions you write on their plan.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            {text("name", "Name", 120, { required: true, autoFocus: true })}
            {text("slug", "Slug", 40, { required: true, placeholder: "used as utm_source" })}
            <div className="sm:col-span-2">{text("url", "URL", 500, { required: true, type: "url", placeholder: "https://" })}</div>
            {area("audience", "Audience", 500)}
            <fieldset className="grid gap-2 sm:col-span-2">
              <legend className="mb-2 text-sm font-medium">Stages accepted</legend>
              <div className="flex flex-wrap gap-4">
                {PROFILE_STAGES.map((s) => (
                  <label key={s} className="flex items-center gap-2 text-sm capitalize">
                    <Checkbox checked={stages.includes(s)} onCheckedChange={(v) => toggleStage(s, v === true)} />
                    {s}
                  </label>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">None ticked means any stage.</p>
            </fieldset>
            <div className="grid gap-2">
              <Label htmlFor="d-fee">Platform fee (USD)</Label>
              <Input id="d-fee" inputMode="decimal" value={form.fee} onChange={(e) => set("fee", e.target.value)} aria-invalid={badCents(cents)} />
            </div>
            {text("fee_note", "Fee note", 300)}
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="d-mode">Who does the final action</Label>
              <Select value={form.mode} onValueChange={(v) => set("mode", v as Destination["mode"])}>
                <SelectTrigger id="d-mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {modes.map((m) => (
                    <SelectItem key={m.value} value={m.value}>
                      {m.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {area("fields", "Form fields", 4000, "What the submission form asks for.")}
            {area("rules", "Platform rules", 4000, "Their current rules in your words, including whether automation or staff submission is allowed.")}
            <div className="grid gap-2">
              <Label htmlFor="d-verified">Last verified</Label>
              <div className="flex gap-2">
                <Input id="d-verified" type="date" value={dayInput(form.last_verified_at)} onChange={(e) => set("last_verified_at", fromDayInput(e.target.value))} />
                <Button type="button" variant="outline" onClick={() => set("last_verified_at", Math.floor(Date.now() / 1000))}>
                  Today
                </Button>
              </div>
            </div>
            <label className="flex items-center gap-2 self-end pb-2 text-sm">
              <Switch checked={form.active} onCheckedChange={(v) => set("active", v)} />
              Active (can be added to plans)
            </label>
            {area("notes", "Notes", 4000)}
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <Button type="submit" disabled={busy || badCents(cents)}>
              {destination ? "Save" : "Add destination"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
