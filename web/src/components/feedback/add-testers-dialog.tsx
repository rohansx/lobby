import { LoaderIcon, UserPlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { api, projectPath } from "@/lib/api"
import { toastError } from "@/lib/errors"

interface Result {
  added: number
  updated: number
  skipped: string[]
}

interface Props {
  slug: string
  onAdded: () => void
}

// Add people by email who may send feedback without joining the waitlist.
export function AddTestersDialog({ slug, onAdded }: Props) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState("")
  const [onlyThese, setOnlyThese] = useState(true)
  const [busy, setBusy] = useState(false)
  const [skipped, setSkipped] = useState<string[]>([])
  const emails = text.split(/[\s,;]+/).filter(Boolean)

  async function add() {
    setBusy(true)
    try {
      const r = await api<Result>(projectPath(slug, "/testers"), { body: { emails, only_these: onlyThese } })
      const parts = []
      if (r.added) parts.push(`${r.added} added`)
      if (r.updated) parts.push(`${r.updated} already on the list, switched on`)
      toast.success(parts.join(", ") || "No one added")
      onAdded()
      setSkipped(r.skipped)
      if (r.skipped.length === 0) {
        setText("")
        setOpen(false)
      }
    } catch (err) {
      toastError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) setSkipped([])
      }}
    >
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <UserPlusIcon />
          Add testers
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add testers</DialogTitle>
          <DialogDescription>
            People who can send feedback from your app without joining the waitlist. Use the email they log in to your app with. Nothing is sent to them, and they won't get waitlist or launch emails.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <Textarea
            rows={6}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder={"ana@example.com\nben@company.com"}
            aria-label="Email addresses, one per line"
          />
          <div className="flex items-start gap-2">
            <Checkbox id="only-these" checked={onlyThese} onCheckedChange={(v) => setOnlyThese(v === true)} />
            <Label htmlFor="only-these" className="text-sm leading-snug font-normal">
              Only people I switch on see the widget. Everyone invited from the waitlist stops seeing it unless you switch them on too.
            </Label>
          </div>
          {skipped.length > 0 && (
            <div className="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs">
              <div className="mb-1 font-medium">Not added</div>
              <ul className="grid gap-0.5">
                {skipped.map((s) => (
                  <li key={s}>{s}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button onClick={add} disabled={busy || emails.length === 0}>
            {busy && <LoaderIcon className="animate-spin" />}
            Add {emails.length > 0 ? emails.length : ""} {emails.length === 1 ? "person" : "people"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
