import { KeyRoundIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Link } from "wouter"
import { CopyField } from "@/components/copy-field"
import { AgentGuideButtons } from "@/components/feedback/agent-guide"
import { SecretKeyDialog } from "@/components/secret-key-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { api, projectPath, type FeedbackList } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { num } from "@/lib/format"

interface Props {
  slug: string
  baseUrl: string
  publicKey: string
  data: FeedbackList
  onChange: () => void
}

const code = "rounded bg-muted px-1 font-mono text-[0.85em]"

export function FeedbackSetup({ slug, baseUrl, publicKey, data, onChange }: Props) {
  const [agentKey, setAgentKey] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const snippet = `<script defer src="${baseUrl}/v1/fb.js" data-project="${publicKey}" data-reporter="REPORTER_TOKEN" data-release="optional"></script>`
  const cli = `LOBBY_URL=${baseUrl} LOBBY_KEY=ak_... lobby feedback next --format md`

  async function setConsole(on: boolean) {
    try {
      await api(projectPath(slug, "/feedback/settings"), { body: { console: on } })
      toast.success(on ? "Console errors are captured" : "Console errors are not captured")
      onChange()
    } catch (err) {
      toastError(err)
    }
  }

  async function setFlag(key: "ai" | "digest" | "screenshots", on: boolean, msg: string) {
    try {
      await api(projectPath(slug, "/feedback/settings"), { body: { [key]: on } })
      toast.success(msg)
      onChange()
    } catch (err) {
      toastError(err)
    }
  }

  async function setAudience(audience: string) {
    try {
      await api(projectPath(slug, "/feedback/settings"), { body: { audience } })
      toast.success(audience === "selected" ? "Only people you switch on see the widget" : "Everyone you invite sees the widget")
      onChange()
    } catch (err) {
      toastError(err)
    }
  }

  async function makeKey() {
    setBusy(true)
    try {
      const r = await api<{ agent_key: string }>(projectPath(slug, "/feedback/agent-key"), { method: "POST" })
      setAgentKey(r.agent_key)
      onChange()
    } catch (err) {
      toastError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Set up</CardTitle>
        <CardDescription>Add the widget to your app for invited users only. They click an element and leave a comment.</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 text-sm">
        <AgentGuideButtons baseUrl={baseUrl} publicKey={publicKey} projectName={slug} />
        <CopyField value={snippet} multiline />
        <div className="grid gap-2 text-xs text-muted-foreground">
          <p>
            Your app's server asks for each logged-in user's reporter token by their email: <code className={code}>POST /v1/reporters {"{\"email\"}"}</code> with the secret key. It also comes back as <code className={code}>reporter_token</code> from <code className={code}>POST /v1/invites/redeem</code>. Tokens last 90 days.
          </p>
          <p>
            Add <code className={code}>data-lobby-id</code> to elements whose markup changes so reports keep grouping. Add <code className={code}>data-lobby-mask</code> to anything whose text must never be captured.
          </p>
          <p>
            Match your brand with <code className={code}>data-accent="#ff5a1f"</code> and <code className={code}>data-accent-ink="#fff"</code>.{" "}
            <code className={code}>data-position="left"</code> moves the button and <code className={code}>data-button</code> renames it.
          </p>
        </div>

        {data.report_limit >= 0 && (
          <p className="text-xs text-muted-foreground">
            <span className="text-foreground tabular-nums">
              {num(data.reports)} / {num(data.report_limit)}
            </span>{" "}
            reports used on the free plan.
          </p>
        )}

        <Separator />
        <div className="flex items-start justify-between gap-3">
          <div>
            <Label htmlFor="fb-ai">Summaries and duplicates</Label>
            <p className="mt-1 text-xs text-muted-foreground">
              {data.ai_available
                ? "Claude reads each issue's reports and screenshots, writes a clear title and summary, and points out issues that look like the same problem. When a report is vague, it asks the reporter one short question in the widget. You decide what to merge. Reports and screenshots are sent to Anthropic to do this."
                : "Not available on this instance: it needs an Anthropic API key (ANTHROPIC_API_KEY)."}
            </p>
          </div>
          <Switch
            id="fb-ai"
            checked={data.ai_available && data.settings.ai}
            disabled={!data.ai_available}
            onCheckedChange={(on) => setFlag("ai", on, on ? "Claude will summarize new reports" : "AI summaries are off")}
          />
        </div>
        <div className="flex items-start justify-between gap-3">
          <div>
            <Label htmlFor="fb-shots">Screenshots</Label>
            <p className="mt-1 text-xs text-muted-foreground">
              The widget captures the area someone picks. Anything marked <code className={code}>data-lobby-mask</code> and every input field is blanked before the image leaves their browser, and they can remove it before sending.
            </p>
          </div>
          <Switch
            id="fb-shots"
            checked={data.settings.screenshots}
            onCheckedChange={(on) => setFlag("screenshots", on, on ? "Screenshots on" : "Screenshots off")}
          />
        </div>
        <div className="flex items-start justify-between gap-3">
          <div>
            <Label htmlFor="fb-digest">Weekly digest</Label>
            <p className="mt-1 text-xs text-muted-foreground">
              An email to the project's notification address in weeks with feedback: the numbers, the top issues{data.ai_available && data.settings.ai ? ", and Claude's read on what to look at first" : ""}.
            </p>
          </div>
          <Switch
            id="fb-digest"
            checked={data.settings.digest}
            onCheckedChange={(on) => setFlag("digest", on, on ? "Weekly digest on" : "Weekly digest off")}
          />
        </div>

        <Separator />
        <div className="grid gap-2">
          <Label htmlFor="fb-audience">Who sees the widget</Label>
          <Select value={data.settings.audience} onValueChange={setAudience}>
            <SelectTrigger id="fb-audience" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="invited">Everyone I've invited</SelectItem>
              <SelectItem value="selected">Only people I switch on</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Switch it on or off per person in the Feedback column on the{" "}
            <Link href={`/p/${slug}/signups`} className="underline">
              Signups page
            </Link>
            . To let in people who aren't on the waitlist, use Add testers there. A switch you set always wins, and the widget checks on every page load, so changes apply at once.
          </p>
        </div>

        <Separator />
        <div className="flex items-start justify-between gap-3">
          <div>
            <Label htmlFor="fb-console">Capture console errors</Label>
            <p className="mt-1 text-xs text-muted-foreground">Attaches recent errors from the browser console to each report.</p>
          </div>
          <Switch id="fb-console" checked={data.settings.console} onCheckedChange={setConsole} />
        </div>

        <Separator />
        <div className="grid gap-2">
          <div className="font-medium">Agent key</div>
          {data.api ? (
            <>
              <p className="text-xs text-muted-foreground">Lets your coding agent read approved issues and leave notes. It can't see open or dismissed issues.</p>
              <Button variant="outline" className="w-fit" disabled={busy} onClick={makeKey}>
                <KeyRoundIcon />
                {data.settings.agent_key_set ? "Rotate agent key" : "Generate agent key"}
              </Button>
              {data.settings.agent_key_set && <p className="text-xs text-muted-foreground">Rotating replaces the current key at once.</p>}
              <CopyField value={cli} multiline />
            </>
          ) : (
            <p className="text-xs text-muted-foreground">
              The feedback API, CLI and agent key come with a <Link href="/campaigns" className="text-foreground underline underline-offset-2">Launch Pack</Link>. Copy as prompt works on every plan.
            </p>
          )}
        </div>
      </CardContent>
      <SecretKeyDialog
        secret={agentKey}
        onClose={() => setAgentKey(null)}
        title="Copy your agent key"
        description={
          <>
            This is the only time it's shown. Use it as <code className="font-mono">LOBBY_KEY</code> for <code className="font-mono">lobby feedback</code>. It only reaches approved issues.
          </>
        }
      />
    </Card>
  )
}
