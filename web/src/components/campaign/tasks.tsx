import { ExternalLinkIcon, LoaderIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { CopyField } from "@/components/copy-field"
import { TaskBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { api, type Campaign, type CampaignTask } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { date } from "@/lib/format"
import { cn } from "@/lib/utils"

type Props = { campaign: Campaign; tasks: CampaignTask[]; onChange: () => Promise<void> }

export function CampaignTasks({ campaign: c, tasks, onChange }: Props) {
  if (!c.approved_at) return <p className="text-sm text-muted-foreground">Tasks start once you approve the plan.</p>
  const yours = tasks.filter((t) => t.owner === "founder")
  const ours = tasks.filter((t) => t.owner !== "founder")
  // Campaign links tag visits and signups with the destination, so the report can tell them apart.
  const link = (t: CampaignTask) => t.campaign_link
  return (
    <div className="grid gap-6">
      {yours.length > 0 && (
        <section className="grid gap-3">
          <h2 className="text-base font-medium">For you to do</h2>
          <p className="text-sm text-muted-foreground">These go out from your own accounts. Post them yourself, then record what happened here.</p>
          {yours.map((t) => (
            <TaskCard key={t.id} task={t} link={link(t)} campaign={c} onChange={onChange} />
          ))}
        </section>
      )}
      {ours.length > 0 && (
        <section className="grid gap-3">
          <h2 className="text-base font-medium">Done by the Lobby team</h2>
          {ours.map((t) => (
            <TaskCard key={t.id} task={t} link={link(t)} campaign={c} onChange={onChange} />
          ))}
        </section>
      )}
    </div>
  )
}

function TaskCard({ task: t, link, campaign, onChange }: { task: CampaignTask; link: string; campaign: Campaign; onChange: () => Promise<void> }) {
  const founder = t.owner === "founder"
  const actionable = founder && campaign.status === "executing" && t.state !== "live"
  return (
    <Card className={cn(founder && t.state === "awaiting_founder" && "border-amber-500/50 bg-amber-500/5")}>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <a href={t.destination_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:underline">
            {t.destination_name}
            <ExternalLinkIcon className="size-3.5 text-muted-foreground" />
          </a>
          <TaskBadge state={t.state} />
        </CardTitle>
        <CardDescription>
          {[t.submitted_at && `Submitted ${date(t.submitted_at)}`, t.live_at && `Live ${date(t.live_at)}`].filter(Boolean).join(" · ") || (founder ? "Not done yet." : "Not submitted yet.")}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 text-sm">
        {founder && t.instructions && (
          <div>
            <div className="mb-1 font-medium">Instructions</div>
            <p className="whitespace-pre-wrap">{t.instructions}</p>
          </div>
        )}
        {t.exception_note && <p className="text-muted-foreground whitespace-pre-wrap">Note: {t.exception_note}</p>}
        {t.listing_url && (
          <a href={t.listing_url} target="_blank" rel="noreferrer" className="w-fit break-all hover:underline">
            {t.listing_url}
          </a>
        )}
        {link && (
          <div className="grid gap-1">
            <div className="text-xs text-muted-foreground">Campaign link: use this wherever the listing asks for your URL</div>
            <CopyField value={link} />
          </div>
        )}
        {actionable && <FounderAction task={t} campaign={campaign} onChange={onChange} />}
      </CardContent>
    </Card>
  )
}

function FounderAction({ task: t, campaign, onChange }: { task: CampaignTask; campaign: Campaign; onChange: () => Promise<void> }) {
  const [url, setUrl] = useState(t.listing_url)
  const [evidence, setEvidence] = useState(t.evidence)
  const [busy, setBusy] = useState("")
  const save = async (state: "submitted" | "live") => {
    setBusy(state)
    try {
      await api(`/campaigns/${campaign.id}/tasks/${t.id}`, { method: "PATCH", body: { state, listing_url: url, evidence } })
      toast.success(state === "live" ? "Marked live." : "Marked submitted.")
      await onChange()
    } catch (err) {
      toastError(err)
    } finally {
      setBusy("")
    }
  }
  const id = `task-${t.id}`
  return (
    <div className="grid gap-3 border-t pt-4">
      <div className="grid gap-2">
        <Label htmlFor={`${id}-url`}>Listing URL</Label>
        <Input id={`${id}-url`} type="url" placeholder="https://" maxLength={500} value={url} onChange={(e) => setUrl(e.target.value)} />
      </div>
      <div className="grid gap-2">
        <Label htmlFor={`${id}-evidence`}>Evidence (optional)</Label>
        <Textarea id={`${id}-evidence`} rows={2} maxLength={4000} placeholder="A confirmation email, a ticket number, anything that shows it went out." value={evidence} onChange={(e) => setEvidence(e.target.value)} />
      </div>
      <div className="flex flex-wrap gap-2">
        {t.state !== "submitted" && (
          <Button variant="outline" disabled={!!busy} onClick={() => save("submitted")}>
            {busy === "submitted" && <LoaderIcon className="animate-spin" />}
            Mark submitted
          </Button>
        )}
        <Button disabled={!!busy || !url.trim()} onClick={() => save("live")}>
          {busy === "live" && <LoaderIcon className="animate-spin" />}
          Mark live
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">Submitted means you sent it; live means the listing is public. Add the listing URL when it's live.</p>
    </div>
  )
}
