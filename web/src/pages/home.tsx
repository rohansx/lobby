import { PlusIcon } from "lucide-react"
import { Link } from "wouter"
import { NewProjectDialog } from "@/components/new-project-dialog"
import { Page } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { num } from "@/lib/format"
import { useSession } from "@/lib/session"

export function Home() {
  const { projects, me } = useSession()
  return (
    <Page
      title="Projects"
      description={me.cloud ? "Your waitlists." : "Every waitlist on this instance."}
      actions={
        <NewProjectDialog>
          <Button>
            <PlusIcon />
            New project
          </Button>
        </NewProjectDialog>
      }
    >
      {me.cloud && me.plan && me.plan.signups > 0 && projects && <FreeUsage used={projects.reduce((n, p) => n + p.total, 0)} limit={me.plan.signups} />}
      {!me.cloud && me.mailer.startsWith("log") && (
        <div className="mb-6 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm">
          Emails are only written to the server log. Set <code className="font-mono">RESEND_API_KEY</code> or <code className="font-mono">SMTP_URL</code>, plus <code className="font-mono">LOBBY_FROM_EMAIL</code>, before you share a waitlist.
        </div>
      )}
      {!projects ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((i) => <Skeleton key={i} className="h-36" />)}
        </div>
      ) : projects.length === 0 ? (
        <Card className="border-dashed">
          <CardHeader className="text-center">
            <CardTitle>No waitlists yet</CardTitle>
            <CardDescription>Create one, then paste its snippet into your site.</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <NewProjectDialog>
              <Button>
                <PlusIcon />
                New project
              </Button>
            </NewProjectDialog>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((p) => (
            <Link key={p.id} href={`/p/${p.slug}`} className="group rounded-xl outline-none focus-visible:ring-3 focus-visible:ring-ring/50">
              <Card className="h-full transition-colors group-hover:border-foreground/20">
                <CardHeader>
                  <CardTitle className="truncate">{p.name}</CardTitle>
                  <CardDescription className="truncate font-mono text-xs">/w/{p.slug}</CardDescription>
                </CardHeader>
                <CardContent className="grid grid-cols-3 gap-2 text-sm">
                  <Metric label="Signups" value={p.total} />
                  <Metric label="Waiting" value={p.confirmed} />
                  <Metric label="Today" value={p.today} />
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </Page>
  )
}

// FreeUsage warns as the Free plan's signup cap gets close.
function FreeUsage({ used, limit }: { used: number; limit: number }) {
  if (used < limit * 0.8) return null
  const full = used >= limit
  return (
    <div className={`mb-6 flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3 text-sm ${full ? "border-destructive/30 bg-destructive/10" : "border-amber-500/30 bg-amber-500/10"}`}>
      <span>
        {full
          ? `Your waitlists have reached the ${num(limit)} signups on the Free plan, so new visitors can't join.`
          : `${num(used)} of ${num(limit)} free signups used.`}{" "}
        Launch has no signup limit.
      </span>
      <Button asChild size="sm">
        <Link href="/billing">Upgrade</Link>
      </Button>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="text-xl font-light tabular-nums tracking-[-0.03em]">{num(value)}</div>
      <div className="text-xs text-muted-foreground">{label}</div>
    </div>
  )
}
