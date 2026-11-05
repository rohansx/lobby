import { FileTextIcon } from "lucide-react"
import { Link, useParams } from "wouter"
import { CampaignPlan, CampaignReadiness } from "@/components/campaign/plan"
import { ProfileTab } from "@/components/campaign/profile"
import { StageSteps } from "@/components/campaign/stage"
import { CampaignTasks } from "@/components/campaign/tasks"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { CampaignBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Campaign, CampaignDetail, CampaignStatus } from "@/lib/api"
import { date } from "@/lib/format"
import { useApi } from "@/lib/use-api"

// The tab that matters most at each stage opens first.
const firstTab: Record<CampaignStatus, string> = {
  intake: "profile",
  readiness: "readiness",
  planning: "plan",
  awaiting_approval: "plan",
  executing: "tasks",
  reporting: "tasks",
  done: "report",
  cancelled: "profile",
}

export default function CampaignPage() {
  const { id } = useParams<{ id: string }>()
  const { data, error, reload } = useApi<CampaignDetail>(`/campaigns/${encodeURIComponent(id)}`)

  if (error && !data) return <LoadError error={error.message === "not found" ? new Error("There's no campaign here.") : error} />
  if (!data) {
    return (
      <Page title="Launch">
        <Skeleton className="h-96" />
      </Page>
    )
  }
  const { campaign: c, tasks } = data
  const founderTodo = tasks.filter((t) => t.owner === "founder" && t.state === "awaiting_founder").length

  return (
    <Page
      title={`Campaign #${c.id}`}
      description={
        <>
          Started {date(c.created_at)}
          {c.project_slug && ` · waitlist ${c.project_slug}`}
        </>
      }
      actions={<CampaignBadge status={c.status} />}
    >
      <StageSteps status={c.status} />
      <Tabs key={c.status} defaultValue={firstTab[c.status]} className="mt-6">
        <TabsList className="flex-wrap">
          <TabsTrigger value="profile">Profile & assets</TabsTrigger>
          <TabsTrigger value="readiness">Readiness</TabsTrigger>
          <TabsTrigger value="plan">Plan{c.status === "awaiting_approval" && " · approve"}</TabsTrigger>
          <TabsTrigger value="tasks">Tasks{founderTodo > 0 && ` · ${founderTodo} for you`}</TabsTrigger>
          <TabsTrigger value="report">Report</TabsTrigger>
        </TabsList>
        <TabsContent value="profile" className="mt-4">
          <ProfileTab campaign={c} onChange={reload} />
        </TabsContent>
        <TabsContent value="readiness" className="mt-4">
          <CampaignReadiness campaign={c} />
        </TabsContent>
        <TabsContent value="plan" className="mt-4">
          <CampaignPlan campaign={c} tasks={tasks} onChange={reload} />
        </TabsContent>
        <TabsContent value="tasks" className="mt-4">
          <CampaignTasks campaign={c} tasks={tasks} onChange={reload} />
        </TabsContent>
        <TabsContent value="report" className="mt-4">
          <ReportTab campaign={c} />
        </TabsContent>
      </Tabs>
    </Page>
  )
}

function ReportTab({ campaign: c }: { campaign: Campaign }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Results report</CardTitle>
        <CardDescription>
          {c.report_published_at
            ? `Published ${date(c.report_published_at)}.`
            : c.approved_at
              ? "Visits and signups from each destination, counted since you approved the plan. The written summary comes when the work is done."
              : "Available once the plan is approved and work starts."}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {c.report_note && <p className="text-sm whitespace-pre-wrap">{c.report_note}</p>}
        {c.approved_at && (
          <Button asChild variant="outline" className="w-fit">
            <Link href={`/campaigns/${c.id}/report`}>
              <FileTextIcon />
              Open the report
            </Link>
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
