import { useParams } from "wouter"
import { AssetGrid } from "@/components/campaign/assets"
import { ProfileForm } from "@/components/campaign/profile-form"
import { LoadError } from "@/components/not-found"
import { CampaignMinutes, ReportEditor, StatusControls } from "@/components/ops/campaign-controls"
import { OpsNav } from "@/components/ops/ops-nav"
import { MAX_TASKS, PlanBuilder } from "@/components/ops/plan-builder"
import { ReadinessEditor } from "@/components/ops/readiness-editor"
import { TaskCard } from "@/components/ops/task-card"
import { Page } from "@/components/page"
import { CampaignBadge } from "@/components/status-badge"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { CampaignStatus, OpsCampaignDetail } from "@/lib/api"
import { date, money } from "@/lib/format"
import { useApi } from "@/lib/use-api"

const firstTab: Record<CampaignStatus, string> = {
  intake: "profile",
  readiness: "readiness",
  planning: "plan",
  awaiting_approval: "plan",
  executing: "plan",
  reporting: "report",
  done: "report",
  cancelled: "profile",
}

export default function OpsCampaignPage() {
  const { id } = useParams<{ id: string }>()
  const { data, error, reload } = useApi<OpsCampaignDetail>(`/ops/campaigns/${encodeURIComponent(id)}`)

  if (error && !data) return <LoadError error={error.message === "not found" ? new Error("There's no campaign here.") : error} />
  if (!data) {
    return (
      <Page title="Operator">
        <Skeleton className="h-96" />
      </Page>
    )
  }
  const { campaign: c, tasks, profile, moves } = data
  const stage = (profile?.approved ?? profile?.latest)?.profile.stage ?? ""
  const taskMinutes = tasks.reduce((n, t) => n + (t.minutes ?? 0), 0)
  const fees = tasks.reduce((n, t) => n + t.fee_cents, 0)

  return (
    <Page
      title={`Campaign #${c.id}`}
      description={
        <>
          {c.owner_email} · {c.project_slug ? `waitlist ${c.project_slug}` : "no waitlist yet"} · paid {money(c.price_cents)}
          {c.paid_at && ` on ${date(c.paid_at)}`}
          {stage && ` · stage ${stage}`}
        </>
      }
      actions={<CampaignBadge status={c.status} ops />}
    >
      <OpsNav />
      <div className="flex flex-wrap items-end justify-between gap-4">
        <StatusControls key={c.updated_at} campaign={c} moves={moves} taskCount={tasks.length} onChange={reload} />
        <CampaignMinutes key={`m-${c.updated_at}`} campaign={c} taskMinutes={taskMinutes} onChange={reload} />
      </div>
      {c.changes_note && c.status === "planning" && (
        <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm">
          <span className="font-medium">The founder asked for changes:</span> <span className="whitespace-pre-wrap">{c.changes_note}</span>
        </div>
      )}
      <Tabs key={c.status} defaultValue={firstTab[c.status]} className="mt-6">
        <TabsList className="flex-wrap">
          <TabsTrigger value="profile">Profile & assets</TabsTrigger>
          <TabsTrigger value="readiness">Readiness</TabsTrigger>
          <TabsTrigger value="plan">
            Plan & tasks · {tasks.length}/{MAX_TASKS}
          </TabsTrigger>
          <TabsTrigger value="report">Report</TabsTrigger>
        </TabsList>
        <TabsContent value="profile" className="mt-4 grid gap-6">
          {c.project_slug ? (
            <>
              <ProfileForm key={c.project_slug} slug={c.project_slug} editable={false} />
              <AssetGrid key={`a-${c.project_slug}`} slug={c.project_slug} editable={false} />
            </>
          ) : (
            <p className="text-sm text-muted-foreground">The founder hasn't chosen a waitlist yet.</p>
          )}
        </TabsContent>
        <TabsContent value="readiness" className="mt-4">
          <ReadinessEditor key={c.updated_at} campaign={c} onChange={reload} />
        </TabsContent>
        <TabsContent value="plan" className="mt-4 grid gap-4">
          {c.status === "planning" && <PlanBuilder campaign={c} tasks={tasks} stage={stage} onChange={reload} />}
          {tasks.length === 0 ? (
            <p className="text-sm text-muted-foreground">{c.status === "planning" ? "No destinations on the plan yet." : "Destinations are added during planning."}</p>
          ) : (
            <p className="text-sm text-muted-foreground">
              {tasks.length} {tasks.length === 1 ? "destination" : "destinations"}, platform fees {money(fees)} (separate from the pack price).
            </p>
          )}
          {tasks.map((t) => (
            <TaskCard key={`${t.id}-${t.updated_at}`} campaign={c} task={t} onChange={reload} />
          ))}
        </TabsContent>
        <TabsContent value="report" className="mt-4">
          <ReportEditor key={c.updated_at} campaign={c} onChange={reload} />
        </TabsContent>
      </Tabs>
    </Page>
  )
}
