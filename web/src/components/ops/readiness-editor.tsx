import { CheckCircle2Icon, CircleAlertIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { api, type OpsCampaign, type ReadinessItem } from "@/lib/api"
import { toastError } from "@/lib/errors"

// The checks from the design; the operator decides which apply.
const standard = ["Landing page", "Demo people can try", "Signup flow works", "Assets", "Tracking tested"]
const MAX_ITEMS = 30 // maxReadinessItems in campaigns.go

export function ReadinessEditor({ campaign: c, onChange }: { campaign: OpsCampaign; onChange: () => Promise<void> }) {
  const [items, setItems] = useState<ReadinessItem[]>(c.readiness)
  const [label, setLabel] = useState("")
  const [busy, setBusy] = useState(false)
  const update = (i: number, patch: Partial<ReadinessItem>) => setItems(items.map((it, j) => (j === i ? { ...it, ...patch } : it)))
  const add = (labels: string[]) => setItems([...items, ...labels.filter((l) => !items.some((it) => it.label === l)).map((l) => ({ label: l, ok: false, note: "" }))].slice(0, MAX_ITEMS))
  const dirty = JSON.stringify(items) !== JSON.stringify(c.readiness)

  async function save() {
    setBusy(true)
    try {
      await api(`/ops/campaigns/${c.id}`, { method: "PATCH", body: { readiness: items } })
      toast.success("Checklist saved")
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
        <CardTitle>Readiness checklist</CardTitle>
        <CardDescription>
          The founder sees every item. Unticked items show as blockers, so write the note as what to fix. {items.filter((i) => !i.ok).length} blockers, {items.length} of {MAX_ITEMS} items.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {items.map((it, i) => (
          <div key={i} className="grid gap-2 rounded-lg border p-3 sm:grid-cols-[auto_1fr_auto]">
            <Button type="button" variant="ghost" size="icon-sm" aria-label={it.ok ? `Mark ${it.label} as a blocker` : `Mark ${it.label} as ok`} aria-pressed={it.ok} onClick={() => update(i, { ok: !it.ok })}>
              {it.ok ? <CheckCircle2Icon className="text-emerald-600" /> : <CircleAlertIcon className="text-destructive" />}
            </Button>
            <div className="grid gap-2">
              <Input aria-label="Label" maxLength={120} value={it.label} onChange={(e) => update(i, { label: e.target.value })} />
              <Textarea aria-label={`Note for ${it.label}`} rows={2} maxLength={500} placeholder="Note" value={it.note} onChange={(e) => update(i, { note: e.target.value })} />
            </div>
            <Button type="button" variant="ghost" size="icon-sm" aria-label={`Remove ${it.label}`} onClick={() => setItems(items.filter((_, j) => j !== i))}>
              <Trash2Icon />
            </Button>
          </div>
        ))}
        <form
          className="flex flex-wrap gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (label.trim()) add([label.trim()])
            setLabel("")
          }}
        >
          <Input className="max-w-64" maxLength={120} placeholder="New item" aria-label="New checklist item" value={label} onChange={(e) => setLabel(e.target.value)} />
          <Button type="submit" variant="outline" disabled={!label.trim() || items.length >= MAX_ITEMS}>
            <PlusIcon />
            Add
          </Button>
          {items.length === 0 && (
            <Button type="button" variant="ghost" onClick={() => add(standard)}>
              Add the standard checks
            </Button>
          )}
        </form>
      </CardContent>
      <CardFooter className="border-t">
        <Button disabled={busy || !dirty || items.some((i) => !i.label.trim())} onClick={save}>
          Save checklist
        </Button>
      </CardFooter>
    </Card>
  )
}
