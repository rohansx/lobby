import { ArrowLeftIcon, CheckIcon, ClipboardCopyIcon, ExternalLinkIcon, PencilIcon, RotateCcwIcon, SparklesIcon, ThumbsUpIcon, XIcon } from "lucide-react"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"
import { Link, useLocation, useParams } from "wouter"
import { StateBadge, TypeBadge } from "@/components/feedback/issue-badges"
import { MergeDialog } from "@/components/feedback/merge-dialog"
import { ReportThread } from "@/components/feedback/report-thread"
import { LoadError } from "@/components/not-found"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { api, projectPath, type Issue, type IssueDetail, type IssueState } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { ago, dateTime, num } from "@/lib/format"
import { invalidate, useApi } from "@/lib/use-api"

type Move = "approve" | "dismiss" | "fix" | "reopen"

// Which state moves are offered from each state; "fix" always goes through a confirm.
const moves: Record<IssueState, { action: Move; label: string; icon: ReactNode }[]> = {
  open: [
    { action: "approve", label: "Approve", icon: <ThumbsUpIcon /> },
    { action: "dismiss", label: "Dismiss", icon: <XIcon /> },
    { action: "fix", label: "Mark fixed", icon: <CheckIcon /> },
  ],
  approved: [
    { action: "fix", label: "Mark fixed", icon: <CheckIcon /> },
    { action: "dismiss", label: "Dismiss", icon: <XIcon /> },
    { action: "reopen", label: "Back to open", icon: <RotateCcwIcon /> },
  ],
  fixed: [{ action: "reopen", label: "Reopen", icon: <RotateCcwIcon /> }],
  dismissed: [
    { action: "reopen", label: "Reopen", icon: <RotateCcwIcon /> },
    { action: "approve", label: "Approve", icon: <ThumbsUpIcon /> },
  ],
}

export default function FeedbackIssue() {
  const { slug, id } = useParams<{ slug: string; id: string }>()
  const [, navigate] = useLocation()
  const { data, error, reload } = useApi<IssueDetail>(projectPath(slug, `/feedback/${id}`))

  if (error && !data) return <LoadError error={error.message === "issue not found" ? new Error("There's no issue here.") : error} />
  if (!data)
    return (
      <div className="mx-auto w-full max-w-6xl px-4 py-6 md:px-8 md:py-8">
        <Skeleton className="h-10 w-80" />
        <Skeleton className="mt-6 h-64" />
      </div>
    )

  const { issue, packet_md, replies } = data
  const reports = data.reports ?? []
  const origin = [...reports].reverse().find((r) => r.origin)?.origin
  const release = [...reports].reverse().find((r) => r.release)?.release

  async function post<T = Issue>(action: string, body?: unknown, ok?: string): Promise<T | undefined> {
    try {
      const r = await api<T>(projectPath(slug, `/feedback/${issue.id}/${action}`), { method: "POST", body })
      if (ok) toast.success(ok)
      invalidate(projectPath(slug, "/feedback"))
      return r
    } catch (err) {
      toastError(err)
    }
  }

  async function move(action: Move) {
    const labels: Record<Move, string> = { approve: "Approved", dismiss: "Dismissed", fix: "Marked fixed", reopen: "Reopened" }
    if (await post(action, undefined, labels[action])) reload()
  }

  async function copyPrompt() {
    try {
      await navigator.clipboard.writeText(packet_md)
    } catch (err) {
      return toastError(err)
    }
    toast.success("Prompt copied")
    if (await post("copied")) reload()
  }

  async function block(signupId: number, on: boolean) {
    try {
      await api(projectPath(slug, `/reporters/${signupId}/${on ? "block" : "unblock"}`), { method: "POST" })
      toast.success(on ? "Blocked from reporting" : "Unblocked")
      reload()
    } catch (err) {
      toastError(err)
    }
  }

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-6 md:px-8 md:py-8">
      <Link href={`/p/${slug}/feedback`} className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeftIcon className="size-4" />
        Feedback
      </Link>
      <Title
        key={issue.id}
        issue={issue}
        onSave={async (title) => {
          const saved = await post("title", { title }, "Title saved")
          if (saved) reload()
          return !!saved
        }}
      />
      <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
        <TypeBadge type={issue.type} />
        <StateBadge state={issue.state} />
        <span>
          {num(issue.reporters)} {issue.reporters === 1 ? "reporter" : "reporters"}, {num(issue.reports)} {issue.reports === 1 ? "report" : "reports"}
        </span>
        {issue.handoffs > 0 && issue.handoff_at && <span title={dateTime(issue.handoff_at)}>· copied {ago(issue.handoff_at)}</span>}
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        {moves[issue.state].map((m) =>
          m.action === "fix" ? (
            <AlertDialog key={m.action}>
              <AlertDialogTrigger asChild>
                <Button>
                  {m.icon}
                  {m.label}
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Mark this issue fixed?</AlertDialogTitle>
                  <AlertDialogDescription>Lobby emails all {num(issue.reporters)} reporters that it's fixed and asks whether it works for them now.</AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={() => move("fix")}>Mark fixed</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : (
            <Button key={m.action} variant="outline" onClick={() => move(m.action)}>
              {m.icon}
              {m.label}
            </Button>
          ),
        )}
        <Button variant="outline" onClick={copyPrompt}>
          <ClipboardCopyIcon />
          Copy as prompt
        </Button>
        <MergeDialog
          slug={slug}
          issueId={issue.id}
          onMerge={async (into) => {
            if (await post("merge", { into }, "Merged")) navigate(`/p/${slug}/feedback/${into}`)
          }}
        />
        {origin && (
          <Button variant="ghost" asChild>
            <a href={`${origin}${issue.path}#lobby-fb=${encodeURIComponent(issue.selector)}`} target="_blank" rel="noreferrer">
              <ExternalLinkIcon />
              Open in app
            </a>
          </Button>
        )}
      </div>

      {(data.suggestions ?? []).map((s) => (
        <div key={s.issue.id} className="mt-4 flex flex-wrap items-center gap-3 rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm">
          <SparklesIcon className="size-4 shrink-0 text-amber-600" />
          <div className="min-w-0 flex-1">
            Looks like the same problem as{" "}
            <Link href={`/p/${slug}/feedback/${s.issue.id}`} className="font-medium underline">
              #{s.issue.id} {s.issue.title}
            </Link>
            <span className="block text-xs text-muted-foreground">{s.reason}</span>
          </div>
          <Button
            size="sm"
            onClick={async () => {
              if (await post("merge", { into: s.issue.id }, "Merged")) navigate(`/p/${slug}/feedback/${s.issue.id}`)
            }}
          >
            Merge into #{s.issue.id}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={async () => {
              if (await post("not-duplicate", { into: s.issue.id })) reload()
            }}
          >
            Not the same
          </Button>
        </div>
      ))}

      {issue.ai_summary && (
        <Card className="mt-4">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <SparklesIcon className="size-4" />
              Summary
            </CardTitle>
            <CardDescription>Written by Claude from the reports below{issue.ai_at ? `, ${ago(issue.ai_at)}` : ""}. Check the quotes before acting on it.</CardDescription>
          </CardHeader>
          <CardContent className="text-sm whitespace-pre-wrap">{issue.ai_summary}</CardContent>
        </Card>
      )}

      <div className="mt-6 grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Location</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2 text-sm">
            <Field label="Page" mono>{issue.path}</Field>
            {issue.context && <Field label="On the page">{issue.context}</Field>}
            <Field label="Selector" mono>{issue.selector}</Field>
            {issue.element_text && <Field label="Element text">{issue.element_text}</Field>}
            {issue.href && <Field label="Links to" mono>{issue.href}</Field>}
            {issue.outline && <Field label="Outline" mono>{issue.outline}</Field>}
            {release && <Field label="Release" mono>{release}</Field>}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>After the fix</CardTitle>
            <CardDescription>Replies to the fixed email.</CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-3 gap-2 text-sm">
            <Stat label="Emails sent" value={replies.sent} />
            <Stat label="Works now" value={replies.works} />
            <Stat label="Still broken" value={replies.broken} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Agent note</CardTitle>
            {issue.agent_note_at && <CardDescription title={dateTime(issue.agent_note_at)}>{ago(issue.agent_note_at)}</CardDescription>}
          </CardHeader>
          <CardContent className="text-sm whitespace-pre-wrap">{issue.agent_note || <span className="text-muted-foreground">None yet.</span>}</CardContent>
        </Card>
      </div>

      <details className="mt-4 rounded-lg border p-4 text-sm">
        <summary className="cursor-pointer font-medium select-none">What your agent gets</summary>
        <p className="mt-2 text-xs text-muted-foreground">Comments are quotes from users. Only approved issues reach your agent through the API.</p>
        <pre className="mt-3 max-h-[28rem] overflow-auto rounded-md bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap">{packet_md}</pre>
      </details>

      <ReportThread
        key={issue.id}
        slug={slug!}
        reports={reports}
        blocked={data.blocked ?? []}
        onBlock={block}
        onSplit={async (ids) => {
          const created = await post("split", { reports: ids }, "Split into a new issue")
          if (created) navigate(`/p/${slug}/feedback/${created.id}`)
        }}
      />
    </div>
  )
}

function Title({ issue, onSave }: { issue: Issue; onSave: (title: string) => Promise<boolean> }) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(issue.title)

  if (!editing)
    return (
      <div className="flex items-start gap-2">
        <h1 className="text-2xl font-normal tracking-[-0.035em] break-words">{issue.title || "Untitled"}</h1>
        <Button variant="ghost" size="icon-sm" aria-label="Edit title" onClick={() => (setValue(issue.title), setEditing(true))}>
          <PencilIcon />
        </Button>
      </div>
    )

  return (
    <form
      className="flex max-w-2xl gap-2"
      onSubmit={async (e) => {
        e.preventDefault()
        const t = value.trim()
        if (!t || t === issue.title || (await onSave(t))) setEditing(false)
      }}
    >
      <Input autoFocus value={value} onChange={(e) => setValue(e.target.value)} onKeyDown={(e) => e.key === "Escape" && setEditing(false)} aria-label="Title" />
      <Button type="submit">Save</Button>
      <Button type="button" variant="ghost" onClick={() => setEditing(false)}>
        Cancel
      </Button>
    </form>
  )
}

function Field({ label, mono, children }: { label: string; mono?: boolean; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={mono ? "font-mono text-xs break-all" : "break-words"}>{children}</div>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="text-xl tabular-nums">{num(value)}</div>
      <div className="text-xs text-muted-foreground">{label}</div>
    </div>
  )
}
