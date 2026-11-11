import { MergeIcon } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { projectPath, type FeedbackList } from "@/lib/api"
import { num } from "@/lib/format"
import { useApi } from "@/lib/use-api"

export function MergeDialog({ slug, issueId, onMerge }: { slug: string; issueId: number; onMerge: (into: number) => Promise<void> }) {
  const [open, setOpen] = useState(false)
  const [into, setInto] = useState("")
  // Fetched once the dialog opens, across every state, so fixed issues can be targets too.
  const { data } = useApi<FeedbackList>(open ? projectPath(slug, "/feedback") : null)
  const others = (data?.data ?? []).filter((i) => i.id !== issueId)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">
          <MergeIcon />
          Merge
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Merge into another issue</DialogTitle>
          <DialogDescription>Every report here moves to the issue you pick, and this one goes away.</DialogDescription>
        </DialogHeader>
        <Select value={into} onValueChange={setInto}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder={data ? (others.length ? "Pick an issue" : "No other issues") : "Loading"} />
          </SelectTrigger>
          <SelectContent>
            {others.map((i) => (
              <SelectItem key={i.id} value={String(i.id)}>
                {i.title || "Untitled"} · {i.state} · {num(i.reporters)} reporters
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <DialogFooter>
          <Button
            disabled={!into}
            onClick={async () => {
              await onMerge(Number(into))
              setOpen(false)
            }}
          >
            Merge
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
