import { Link } from "wouter"
import { OpsNav } from "@/components/ops/ops-nav"
import { Page } from "@/components/page"
import { CampaignBadge, TaskBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import type { OpsCampaign, TaskState } from "@/lib/api"
import { ago, dateTime, num } from "@/lib/format"
import { useApi } from "@/lib/use-api"

// What the operator has to do next, or "" when the campaign is waiting on the founder or finished.
function operatorTodo(c: OpsCampaign) {
  switch (c.status) {
    case "readiness":
      return "Review readiness"
    case "planning":
      return c.changes_note ? "Changes requested" : "Build the plan"
    case "executing":
      return c.task_states?.blocked ? "Blocked tasks" : "Run tasks"
    case "reporting":
      return "Write the report"
  }
  return ""
}

// Lower first: operator action needed, then work in progress, then waiting on the founder, then closed.
function rank(c: OpsCampaign) {
  const order: Record<string, number> = { "Review readiness": 0, "Changes requested": 1, "Build the plan": 2, "Blocked tasks": 3, "Run tasks": 4, "Write the report": 5 }
  return order[operatorTodo(c)] ?? { intake: 6, awaiting_approval: 7, done: 8, cancelled: 9 }[c.status as string] ?? 9
}

const stateOrder: TaskState[] = ["blocked", "rejected", "needs_assets", "awaiting_approval", "awaiting_founder", "planned", "ready", "submitted", "live"]

export default function OpsCampaigns() {
  const { data } = useApi<{ data: OpsCampaign[] }>("/ops/campaigns")
  // Oldest change first within a rank, so nothing waits forever.
  const rows = [...(data?.data ?? [])].sort((a, b) => rank(a) - rank(b) || a.updated_at - b.updated_at)

  return (
    <Page title="Operator" description="Launch Pack campaigns, the ones waiting on you first.">
      <OpsNav />
      {!data ? (
        <Skeleton className="h-40" />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">Campaign</TableHead>
                <TableHead>Account</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Tasks</TableHead>
                <TableHead className="text-right">Minutes</TableHead>
                <TableHead className="pr-4">Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="py-10 text-center text-muted-foreground">
                    No campaigns yet. They open when a pack is paid, or from Accounts.
                  </TableCell>
                </TableRow>
              )}
              {rows.map((c) => {
                const todo = operatorTodo(c)
                return (
                  <TableRow key={c.id}>
                    <TableCell className="pl-4">
                      <Link href={`/ops/campaigns/${c.id}`} className="font-medium hover:underline">
                        #{c.id} {c.project_slug || <span className="text-muted-foreground">no waitlist</span>}
                      </Link>
                    </TableCell>
                    <TableCell className="max-w-56 truncate">{c.owner_email}</TableCell>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-1.5">
                        <CampaignBadge status={c.status} ops />
                        {todo && <Badge variant={todo === "Blocked tasks" || todo === "Changes requested" ? "destructive" : "outline"}>{todo}</Badge>}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {c.tasks === 0 && <span className="text-muted-foreground">none</span>}
                        {stateOrder
                          .filter((s) => c.task_states?.[s])
                          .map((s) => (
                            <span key={s} className="inline-flex items-center gap-1 text-xs">
                              <TaskBadge state={s} ops />
                              <span className="tabular-nums">{c.task_states?.[s]}</span>
                            </span>
                          ))}
                      </div>
                    </TableCell>
                    <TableCell className="text-right tabular-nums" title="Campaign minutes + task minutes">
                      {num((c.operator_minutes ?? 0) + (c.task_minutes ?? 0))}
                    </TableCell>
                    <TableCell className="pr-4 text-muted-foreground" title={dateTime(c.updated_at)}>
                      {ago(c.updated_at)}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}
    </Page>
  )
}
