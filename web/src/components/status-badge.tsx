import type { ReactNode } from "react"
import { Badge } from "@/components/ui/badge"
import { campaignLabel, type CampaignStatus, type Status, type TaskState } from "@/lib/api"
import { cn } from "@/lib/utils"

const amber = "bg-amber-500/10 text-amber-700 dark:text-amber-300"
const sky = "bg-sky-500/10 text-sky-700 dark:text-sky-300"
const violet = "bg-violet-500/10 text-violet-700 dark:text-violet-300"
const emerald = "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
const red = "bg-destructive/10 text-destructive"
const muted = "bg-muted text-muted-foreground"

const tone: Record<Status, string> = { pending: amber, confirmed: sky, invited: violet, joined: emerald, removed: muted }

export function ToneBadge({ className, children }: { className: string; children: ReactNode }) {
  return (
    <Badge variant="secondary" className={cn("capitalize", className)}>
      {children}
    </Badge>
  )
}

export function StatusBadge({ status }: { status: Status }) {
  return <ToneBadge className={tone[status]}>{status}</ToneBadge>
}

const campaignTone: Record<CampaignStatus, string> = {
  intake: amber,
  readiness: sky,
  planning: sky,
  awaiting_approval: amber,
  executing: violet,
  reporting: violet,
  done: emerald,
  cancelled: muted,
}

export function CampaignBadge({ status, ops }: { status: CampaignStatus; ops?: boolean }) {
  return <ToneBadge className={campaignTone[status]}>{ops && status === "awaiting_approval" ? "Founder approval" : campaignLabel[status]}</ToneBadge>
}

const taskTone: Record<TaskState, string> = {
  planned: muted,
  needs_assets: amber,
  awaiting_approval: amber,
  awaiting_founder: amber,
  ready: sky,
  submitted: violet,
  live: emerald,
  rejected: red,
  blocked: red,
}

const taskLabel: Partial<Record<TaskState, string>> = { awaiting_founder: "your action", live: "accepted / live" }

// ops shows the plain state names instead of the founder's wording.
export function TaskBadge({ state, ops }: { state: TaskState; ops?: boolean }) {
  return <ToneBadge className={taskTone[state]}>{(!ops && taskLabel[state]) || state.replaceAll("_", " ")}</ToneBadge>
}
