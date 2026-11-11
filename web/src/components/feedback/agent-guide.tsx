import { BotIcon, CheckIcon, DownloadIcon } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import guide from "../../../../docs/feedback-integration.md?raw"

interface Props {
  baseUrl: string
  publicKey: string
  projectName: string
}

// The guide from docs/, with this waitlist's public values filled in. The
// secret key is never included: the founder adds it to their server's env.
function agentGuide({ baseUrl, publicKey, projectName }: Props): string {
  const header = [
    `<!-- Lobby Feedback setup for "${projectName}" -->`,
    "",
    "## Values for this app",
    "",
    "```sh",
    `LOBBY_URL=${baseUrl}`,
    `LOBBY_PUBLIC_KEY=${publicKey}`,
    "LOBBY_SECRET_KEY=sk_...   # server only; ask the founder, never commit it",
    "```",
    "",
  ].join("\n")
  const body = guide
    .replaceAll("{LOBBY_URL}", baseUrl)
    .replaceAll("{LOBBY_PUBLIC_KEY}", publicKey)
    .replaceAll("https://trylobby.app", baseUrl)
  // Put the values right after the title, where an agent reads first.
  const nl = body.indexOf("\n")
  return body.slice(0, nl + 1) + "\n" + header + body.slice(nl + 1)
}

export function AgentGuideButtons(props: Props) {
  const [copied, setCopied] = useState(false)
  const text = agentGuide(props)
  return (
    <div className="grid gap-2 rounded-lg border bg-muted/30 p-3">
      <div>
        <div className="font-medium">Let your coding agent set it up</div>
        <p className="mt-1 text-xs text-muted-foreground">
          A guide for the agent in your app's repo, with this waitlist's URL and public key filled in. Add the secret key to your server's environment yourself.
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          onClick={async () => {
            await navigator.clipboard.writeText(text)
            setCopied(true)
            setTimeout(() => setCopied(false), 2000)
          }}
        >
          {copied ? <CheckIcon /> : <BotIcon />}
          {copied ? "Copied" : "Copy guide for your agent"}
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => {
            const url = URL.createObjectURL(new Blob([text], { type: "text/markdown" }))
            const a = Object.assign(document.createElement("a"), { href: url, download: "lobby-feedback-integration.md" })
            a.click()
            URL.revokeObjectURL(url)
          }}
        >
          <DownloadIcon />
          Download .md
        </Button>
      </div>
    </div>
  )
}
