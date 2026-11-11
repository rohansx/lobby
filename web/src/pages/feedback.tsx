import { useState } from "react"
import { Link, useParams } from "wouter"
import { FeedbackSetup } from "@/components/feedback/setup-card"
import { StateBadge, TypeBadge } from "@/components/feedback/issue-badges"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ISSUE_STATES, ISSUE_TYPES, projectPath, type FeedbackList } from "@/lib/api"
import { ago, dateTime, num } from "@/lib/format"
import { useSession } from "@/lib/session"
import { useApi } from "@/lib/use-api"
import { useProject } from "@/lib/use-project"

const ALL = "all"

export default function Feedback() {
  const { slug } = useParams<{ slug: string }>()
  const { me } = useSession()
  const project = useProject()
  const [state, setState] = useState("open")
  const [type, setType] = useState(ALL)
  const [wave, setWave] = useState(ALL)
  const [source, setSource] = useState(ALL)

  const params = new URLSearchParams()
  for (const [k, v] of [["state", state], ["type", type], ["wave", wave], ["source", source]]) if (v !== ALL) params.set(k, v)
  const { data, error, reload } = useApi<FeedbackList>(projectPath(slug, `/feedback?${params}`))

  if (error && !data) return <LoadError error={error} />

  const issues = data?.data ?? []
  const waves = data?.waves ?? []
  const sources = data?.sources ?? []

  return (
    <Page title="Feedback" description="Comments invited users leave on your live app, grouped by element and page. Most reporters first.">
      <div className="grid gap-4 lg:grid-cols-[1fr_22rem] lg:items-start">
        <div className="min-w-0">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Tabs value={state} onValueChange={setState}>
              <TabsList>
                <TabsTrigger value={ALL}>All</TabsTrigger>
                {ISSUE_STATES.map((s) => (
                  <TabsTrigger key={s} value={s} className="capitalize">
                    {s}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
            <Filter value={type} onChange={setType} all="All types" options={ISSUE_TYPES} capitalize />
            {waves.length > 0 && <Filter value={wave} onChange={setWave} all="All waves" options={waves} />}
            {sources.length > 0 && <Filter value={source} onChange={setSource} all="All sources" options={sources} />}
          </div>

          {!data ? (
            <Skeleton className="h-64" />
          ) : data.reports === 0 ? (
            <Card className="border-dashed">
              <CardHeader className="text-center">
                <CardTitle>No feedback yet</CardTitle>
                <CardDescription>Add the script from Set up to your app, only for invited users, with each user's reporter token. Their reports show up here, grouped into issues.</CardDescription>
              </CardHeader>
            </Card>
          ) : (
            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="pl-4">Issue</TableHead>
                    <TableHead>State</TableHead>
                    <TableHead className="text-right">Reporters</TableHead>
                    <TableHead className="hidden md:table-cell">Page</TableHead>
                    <TableHead>Last activity</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {issues.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={5} className="py-10 text-center text-muted-foreground">
                        No issues match.
                      </TableCell>
                    </TableRow>
                  )}
                  {issues.map((i) => (
                    <TableRow key={i.id}>
                      <TableCell className="max-w-64 pl-4">
                        <Link href={`/p/${slug}/feedback/${i.id}`} className="flex min-w-0 items-center gap-2 hover:underline">
                          <TypeBadge type={i.type} />
                          <span className="truncate">{i.title || "Untitled"}</span>
                        </Link>
                      </TableCell>
                      <TableCell>
                        <StateBadge state={i.state} />
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{num(i.reporters)}</TableCell>
                      <TableCell className="hidden max-w-36 truncate font-mono text-xs text-muted-foreground md:table-cell">{i.path}</TableCell>
                      <TableCell className="text-muted-foreground" title={dateTime(i.updated_at)}>
                        {ago(i.updated_at)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </div>

        {data && project.data ? (
          <FeedbackSetup slug={slug} baseUrl={me.base_url} publicKey={project.data.project.public_key} data={data} onChange={reload} />
        ) : (
          <Skeleton className="h-96" />
        )}
      </div>
    </Page>
  )
}

function Filter({ value, onChange, all, options, capitalize }: { value: string; onChange: (v: string) => void; all: string; options: string[]; capitalize?: boolean }) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="w-36">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>{all}</SelectItem>
        {options.map((o) => (
          <SelectItem key={o} value={o} className={capitalize ? "capitalize" : undefined}>
            {o}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
