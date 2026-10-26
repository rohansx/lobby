import { Badge } from "@/components/ui/badge"
import type { Status } from "@/lib/api"
import { cn } from "@/lib/utils"

const tone: Record<Status, string> = {
  pending: "bg-amber-500/10 text-amber-700 dark:text-amber-300",
  confirmed: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  invited: "bg-violet-500/10 text-violet-700 dark:text-violet-300",
  joined: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  removed: "bg-muted text-muted-foreground",
}

export function StatusBadge({ status }: { status: Status }) {
  return (
    <Badge variant="secondary" className={cn("capitalize", tone[status])}>
      {status}
    </Badge>
  )
}
