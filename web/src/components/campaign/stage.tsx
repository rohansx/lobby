import { CheckIcon } from "lucide-react"
import { campaignLabel, type CampaignStatus } from "@/lib/api"
import { cn } from "@/lib/utils"

const steps: CampaignStatus[] = ["intake", "readiness", "planning", "awaiting_approval", "executing", "reporting", "done"]

const next: Record<CampaignStatus, string> = {
  intake: "Your turn: choose the waitlist, fill in the product profile, upload assets, then submit for the readiness review.",
  readiness: "The Lobby team is reviewing your landing page, demo, signup flow and assets.",
  planning: "The Lobby team is building your shortlist of destinations. You'll get an email when it's ready to approve.",
  awaiting_approval: "Your turn: review the plan and approve it, or ask for changes.",
  executing: "Submissions are under way. Tasks marked for you need your action.",
  reporting: "The work is done and the report is being written.",
  done: "The campaign is complete. The report is ready.",
  cancelled: "This campaign was cancelled.",
}

export function StageSteps({ status }: { status: CampaignStatus }) {
  const at = steps.indexOf(status)
  return (
    <div className="grid gap-3">
      {status !== "cancelled" && (
        <ol className="flex flex-wrap gap-x-1 gap-y-2 text-xs" aria-label="Campaign stages">
          {steps.map((s, i) => (
            <li key={s} aria-current={i === at ? "step" : undefined} className={cn("flex items-center gap-1.5 rounded-full border px-2.5 py-1", i === at ? "border-foreground bg-foreground text-background" : i < at ? "text-foreground" : "text-muted-foreground")}>
              {i < at ? <CheckIcon className="size-3" /> : <span className="tabular-nums">{i + 1}</span>}
              {campaignLabel[s]}
            </li>
          ))}
        </ol>
      )}
      <p className="text-sm text-muted-foreground">{next[status]}</p>
    </div>
  )
}
