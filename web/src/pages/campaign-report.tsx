import { ArrowLeftIcon, PrinterIcon } from "lucide-react"
import { Link, useLocation, useParams } from "wouter"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import type { CampaignReport, ReportRow, TaskState } from "@/lib/api"
import { date, money, num, pct } from "@/lib/format"
import { useApi } from "@/lib/use-api"

// Plain outcome wording: anything that hasn't reached a result yet is pending.
function outcome(s?: TaskState) {
  if (s === "live") return "Live"
  if (s === "submitted") return "Submitted, not live yet"
  if (s === "rejected") return "Rejected"
  if (s === "blocked") return "Blocked"
  return "Pending"
}

const muted = (s: string) => <span className="text-muted-foreground">{s}</span>

function Numbers({ r }: { r: ReportRow }) {
  return (
    <>
      <TableCell className="text-right tabular-nums">{r.visits === null ? muted("not recorded") : num(r.visits)}</TableCell>
      <TableCell className="text-right tabular-nums">{num(r.signups)}</TableCell>
      <TableCell className="text-right tabular-nums">{num(r.confirmed)}</TableCell>
      <TableCell className="text-right tabular-nums">{num(r.relevant)}</TableCell>
      <TableCell className="text-right tabular-nums">
        {r.conversion === null ? muted("–") : `${pct(r.conversion)} (${num(r.signups)} of ${num(r.visits)})`}
      </TableCell>
    </>
  )
}

export default function CampaignReportPage() {
  const { id } = useParams<{ id: string }>()
  const [location] = useLocation()
  const ops = location.startsWith("/ops/")
  const back = `${ops ? "/ops" : ""}/campaigns/${encodeURIComponent(id)}`
  const { data: rep, error } = useApi<CampaignReport>(`${back}/report`)

  if (error && !rep) return <LoadError error={error.message === "not found" ? new Error("There's no campaign here.") : error} />
  if (!rep) {
    return (
      <Page title="Results report">
        <Skeleton className="h-96" />
      </Page>
    )
  }
  const c = rep.campaign
  const span = rep.from
    ? `${date(rep.from)} to ${rep.campaign.report_published_at ? date(rep.to) : `${date(rep.to)} (still counting)`}`
    : "starts when the plan is approved"

  return (
    <Page
      title={`Results report · Campaign #${c.id}`}
      description={
        <>
          {c.project_slug ? `Waitlist ${c.project_slug}` : "No waitlist chosen"} · Counting window {span}
        </>
      }
      actions={
        <div className="flex gap-2 print:hidden">
          <Button asChild variant="ghost">
            <Link href={back}>
              <ArrowLeftIcon />
              Campaign
            </Link>
          </Button>
          <Button variant="outline" onClick={() => print()}>
            <PrinterIcon />
            Print
          </Button>
        </div>
      }
    >
      <div className="grid gap-6">
        {!rep.from && <p className="text-sm text-muted-foreground">Nothing is counted yet. Counting starts when the plan is approved.</p>}
        {c.report_note && (
          <Card>
            <CardHeader>
              <CardTitle>Summary{!c.report_published_at && " (draft, not published)"}</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm whitespace-pre-wrap">{c.report_note}</p>
            </CardContent>
          </Card>
        )}

        <Card className="overflow-hidden py-0 print:overflow-visible">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">Destination</TableHead>
                <TableHead>Outcome</TableHead>
                <TableHead className="text-right">Visits</TableHead>
                <TableHead className="text-right">Signups</TableHead>
                <TableHead className="text-right">Confirmed</TableHead>
                <TableHead className="text-right">Relevant</TableHead>
                <TableHead className="text-right">Signups per visit</TableHead>
                <TableHead className="pr-4 text-right">Platform fee</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rep.rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="py-8 text-center text-muted-foreground">
                    No destinations on the plan yet.
                  </TableCell>
                </TableRow>
              )}
              {rep.rows.map((r) => (
                <TableRow key={r.task_id}>
                  <TableCell className="pl-4 align-top whitespace-normal">
                    <div className="font-medium">{r.label}</div>
                    <div className="text-xs text-muted-foreground">
                      {r.owner === "founder" ? (ops ? "Founder" : "Done by you") : "Done by the Lobby team"} · source tag {r.source}
                    </div>
                  </TableCell>
                  <TableCell className="align-top whitespace-normal">
                    <div>{outcome(r.state)}</div>
                    <div className="text-xs text-muted-foreground">
                      {r.submitted_at && `Submitted ${date(r.submitted_at)}`}
                      {r.submitted_at && r.live_at && " · "}
                      {r.live_at && `Live ${date(r.live_at)}`}
                    </div>
                    {r.listing_url && (
                      <a href={r.listing_url} target="_blank" rel="noreferrer" className="block max-w-56 truncate text-xs underline print:max-w-none print:whitespace-normal">
                        {r.listing_url}
                      </a>
                    )}
                  </TableCell>
                  <Numbers r={r} />
                  <TableCell className="pr-4 text-right tabular-nums">{r.fee_cents ? money(r.fee_cents) : muted("none")}</TableCell>
                </TableRow>
              ))}
              {[rep.unknown, rep.other].map((r) => (
                <TableRow key={r.kind}>
                  <TableCell className="pl-4 whitespace-normal">
                    <div className="font-medium">{r.label}</div>
                    <div className="text-xs text-muted-foreground">
                      {r.kind === "unknown" ? "No source tag: direct visits, your own links, the widget or forms" : "Tagged with a source that isn't on the plan"}
                    </div>
                  </TableCell>
                  <TableCell />
                  <Numbers r={r} />
                  <TableCell />
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell className="pl-4 font-medium">{rep.total.label}</TableCell>
                <TableCell />
                <Numbers r={rep.total} />
                <TableCell className="pr-4 text-right tabular-nums">{money(rep.platform_fees_cents)}</TableCell>
              </TableRow>
            </TableFooter>
          </Table>
        </Card>

        <div className="grid gap-6 md:grid-cols-2 print:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Cost</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-2 text-sm">
              <Line label="Launch Pack" value={money(rep.pack_price_cents)} />
              <Line label="Platform fees on the plan (paid to the platforms, separate from the pack)" value={money(rep.platform_fees_cents)} />
              <Line
                label={`Cost per confirmed signup from plan destinations (${num(rep.attributed_confirmed)} confirmed)`}
                value={rep.cost_per_confirmed_cents === null ? "not shown until one is confirmed" : money(rep.cost_per_confirmed_cents)}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>How this is counted</CardTitle>
            </CardHeader>
            <CardContent>
              <ul className="grid list-disc gap-1.5 pl-4 text-sm text-muted-foreground">
                <li>Visits are page views of your hosted waitlist page, counted per day. Bots, link previews and browser prefetches are left out. Visits to your own site, the widget and plain forms are not counted.</li>
                <li>"Not recorded" means no visits were counted at all in this window, so there is nothing to compare against. It is not the same as zero.</li>
                <li>A signup belongs to the source tag it arrived with, from the campaign link for that destination. People who came another way, or opened the link and signed up later from somewhere else, land under unknown or other sources.</li>
                <li>Each address counts once. Removed signups are left out. Confirmed means the person clicked the link in their confirmation email. Relevant is your own review on the signups page.</li>
                <li>Signups per visit is only shown where visits were counted. Small numbers move a lot, so read it next to the counts.</li>
                <li>This shows what was recorded, not what caused it. Some listings are still pending review and may bring visits later.</li>
              </ul>
            </CardContent>
          </Card>
        </div>
      </div>
    </Page>
  )
}

function Line({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="shrink-0 font-medium tabular-nums">{value}</span>
    </div>
  )
}
