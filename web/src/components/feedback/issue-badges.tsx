import { Badge } from "@/components/ui/badge"
import type { IssueState, IssueType } from "@/lib/api"
import { cn } from "@/lib/utils"

const typeLabel: Record<IssueType, string> = { bug: "Bug", confusing: "Confusing", idea: "Idea" }

const stateStyle: Record<IssueState, string> = {
  open: "bg-amber-500",
  approved: "bg-sky-500",
  fixed: "bg-emerald-500",
  dismissed: "bg-muted-foreground/50",
}

export function TypeBadge({ type }: { type: IssueType }) {
  return <Badge variant={type === "bug" ? "destructive" : "secondary"}>{typeLabel[type] ?? type}</Badge>
}

export function StateBadge({ state }: { state: IssueState }) {
  return (
    <Badge variant="outline" className="capitalize">
      <span className={cn("size-1.5 rounded-full", stateStyle[state])} />
      {state}
    </Badge>
  )
}
