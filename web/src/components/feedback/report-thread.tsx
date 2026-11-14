import { BanIcon, SplitIcon } from "lucide-react"
import { useState } from "react"
import { TypeBadge } from "@/components/feedback/issue-badges"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { projectPath, type FeedbackReport } from "@/lib/api"
import { dateTime, num } from "@/lib/format"

interface Props {
  slug: string
  reports: FeedbackReport[]
  blocked: number[]
  onBlock: (signupId: number, block: boolean) => void
  onSplit: (reportIds: number[]) => void
}

export function ReportThread({ slug, reports, blocked, onBlock, onSplit }: Props) {
  const [selected, setSelected] = useState<number[]>([])
  // Splitting every report would leave this issue empty.
  const canSplit = selected.length > 0 && selected.length < reports.length

  return (
    <section>
      <div className="mt-8 mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-medium">Reports, oldest first</h2>
        {selected.length > 0 && (
          <Button size="sm" variant="outline" disabled={!canSplit} onClick={() => onSplit(selected)}>
            <SplitIcon />
            Split {num(selected.length)} into a new issue
          </Button>
        )}
      </div>
      <div className="grid gap-3">
        {reports.map((r) => {
          const isBlocked = blocked.includes(r.signup_id)
          const errors = r.console_errors ?? []
          const failed = r.failed_requests ?? []
          return (
            <Card key={r.id} className="py-4">
              <CardContent className="grid gap-3 px-4">
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <Checkbox
                    aria-label="Select report"
                    checked={selected.includes(r.id)}
                    onCheckedChange={(c) => setSelected((cur) => (c ? [...cur, r.id] : cur.filter((x) => x !== r.id)))}
                  />
                  <span className="font-medium">{r.email || `Signup ${r.signup_id}`}</span>
                  <TypeBadge type={r.type} />
                  <span className="text-xs text-muted-foreground">
                    {[r.source, r.wave && `wave ${r.wave}`, dateTime(r.created_at)].filter(Boolean).join(" · ")}
                  </span>
                  <Button size="sm" variant="ghost" className="ml-auto" onClick={() => onBlock(r.signup_id, !isBlocked)}>
                    <BanIcon />
                    {isBlocked ? "Unblock" : "Block"}
                  </Button>
                </div>
                {/* Rendered as text: comments are untrusted user input. */}
                <p className="text-sm whitespace-pre-wrap">{r.comment}</p>
                {r.followup_question && (
                  <div className="grid gap-1 rounded-md border-l-2 border-muted-foreground/30 pl-3 text-sm">
                    <span className="text-xs text-muted-foreground">Claude asked: {r.followup_question}</span>
                    <p className="whitespace-pre-wrap">{r.followup_answer || <span className="text-muted-foreground">No answer.</span>}</p>
                  </div>
                )}
                {r.has_screenshot && <Screenshot src={`/api/admin${projectPath(slug, `/feedback/reports/${r.id}/screenshot`)}`} />}
                <details className="text-xs text-muted-foreground">
                  <summary className="cursor-pointer select-none">
                    Details{errors.length + failed.length > 0 && `, ${errors.length} console errors, ${failed.length} failed requests`}
                  </summary>
                  <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                    <dt>Viewport</dt>
                    <dd>{r.viewport || "–"}</dd>
                    <dt>Browser</dt>
                    <dd className="break-all">{r.browser || "–"}</dd>
                    {r.release && (
                      <>
                        <dt>Release</dt>
                        <dd className="font-mono">{r.release}</dd>
                      </>
                    )}
                  </dl>
                  {errors.length > 0 && <pre className="mt-2 overflow-x-auto rounded-md bg-muted/40 p-2 font-mono">{errors.join("\n")}</pre>}
                  {failed.length > 0 && (
                    <ul className="mt-2 font-mono">
                      {failed.map((f, i) => (
                        <li key={i}>
                          {f.status} {f.path}
                        </li>
                      ))}
                    </ul>
                  )}
                </details>
              </CardContent>
            </Card>
          )
        })}
      </div>
    </section>
  )
}

function Screenshot({ src }: { src: string }) {
  return (
    <a href={src} target="_blank" rel="noreferrer" className="block w-fit max-w-full" title="Open full size">
      <img src={src} loading="lazy" alt="Screenshot of the area the reporter picked" className="max-h-64 max-w-full rounded-md border object-contain object-left-top" />
      <span className="mt-1 block text-xs text-muted-foreground">Screenshot of the picked area. Grey blocks are parts your app hides.</span>
    </a>
  )
}
