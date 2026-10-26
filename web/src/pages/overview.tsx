import { DownloadIcon, ExternalLinkIcon, KeyRoundIcon, SendIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { CopyField } from "@/components/copy-field"
import { DailyChart } from "@/components/daily-chart"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { SecretKeyDialog } from "@/components/secret-key-dialog"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { api, projectPath } from "@/lib/api"
import { num, pct } from "@/lib/format"
import { useSession } from "@/lib/session"
import { useProject } from "@/lib/use-project"

export default function Overview() {
  const { slug, data, error, reload } = useProject()
  const { refreshProjects } = useSession()
  const [count, setCount] = useState("10")
  const [inviting, setInviting] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)

  if (error && !data) return <LoadError error={error} />
  if (!data)
    return (
      <Page title=" ">
        <div className="grid gap-4 md:grid-cols-4">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-24" />)}</div>
        <Skeleton className="mt-4 h-64" />
      </Page>
    )

  const { project: p, stats: st, snippets } = data
  const waiting = st.by_status.confirmed ?? 0
  const exportBase = `/api/admin${projectPath(slug)}/export`

  return (
    <Page
      title={p.name}
      description={
        <a href={snippets.hosted} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:underline">
          {snippets.hosted.replace(/^https?:\/\//, "")}
          <ExternalLinkIcon className="size-3" />
        </a>
      }
      actions={
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline">
              <DownloadIcon />
              Export
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem asChild>
              <a href={`${exportBase}/csv`}>CSV, confirmed and later</a>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <a href={`${exportBase}/csv?status=all`}>CSV, everyone</a>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <a href={`${exportBase}/ndjson?status=all`}>NDJSON, everyone</a>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      }
    >
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label="Signups" value={num(st.total)} hint={`${num(st.by_status.pending)} unconfirmed`} />
        <StatCard label="Waiting" value={num(waiting)} hint="Confirmed, not yet invited" />
        <StatCard label="Confirm rate" value={pct(st.confirm_rate)} hint={p.doi_enabled ? "Double opt-in on" : "Double opt-in off"} />
        <StatCard label="Invited / joined" value={`${num(st.by_status.invited)} / ${num(st.by_status.joined)}`} hint={`${num(st.referred)} came by referral`} />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Signups per day</CardTitle>
            <CardDescription>Last 30 days, UTC</CardDescription>
          </CardHeader>
          <CardContent>
            <DailyChart daily={st.daily ?? []} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Invite the next wave</CardTitle>
            <CardDescription>Emails the front of the line a magic link and a code bound to their address.</CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="flex gap-2"
              onSubmit={async (e) => {
                e.preventDefault()
                setInviting(true)
                try {
                  const r = await api<{ count: number }>(projectPath(slug, "/invite"), { body: { count: Number(count) } })
                  toast.success(r.count ? `${num(r.count)} people invited` : "Nobody is waiting to be invited")
                  reload()
                  refreshProjects()
                } catch (err) {
                  toast.error((err as Error).message)
                } finally {
                  setInviting(false)
                }
              }}
            >
              <Input type="number" min={1} max={1000} value={count} onChange={(e) => setCount(e.target.value)} className="w-24" aria-label="How many" />
              <Button type="submit" disabled={inviting || waiting === 0} className="flex-1">
                <SendIcon />
                Invite next {count || 0}
              </Button>
            </form>
            <p className="mt-3 text-xs text-muted-foreground">{num(waiting)} confirmed people are waiting.</p>
          </CardContent>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <RankCard title="Top sources" empty="No UTM sources or referrers yet." rows={(st.top_sources ?? []).map((s) => ({ key: s.name, label: s.name, count: s.count }))} />
        <RankCard title="Top referrers" empty="Nobody has a credited referral yet." rows={(st.top_referrers ?? []).map((r) => ({ key: String(r.id), label: r.email, count: r.count }))} />
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle>Add it to a site</CardTitle>
          <CardDescription>
            Pick one. Signups from other sites are refused unless their origin is listed in Settings.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="script">
            <TabsList>
              <TabsTrigger value="script">Script widget</TabsTrigger>
              <TabsTrigger value="form">Plain form</TabsTrigger>
              <TabsTrigger value="hosted">Hosted page</TabsTrigger>
            </TabsList>
            <TabsContent value="script" className="mt-3 grid gap-2">
              <CopyField value={snippets.script} multiline />
              <p className="text-xs text-muted-foreground">Under 2 KB gzipped. Theme it with --wl-accent, --wl-radius and friends on the container.</p>
            </TabsContent>
            <TabsContent value="form" className="mt-3 grid gap-2">
              <CopyField value={snippets.form} multiline />
              <p className="text-xs text-muted-foreground">No JavaScript. Visitors land on their status page or your redirect URL.</p>
            </TabsContent>
            <TabsContent value="hosted" className="mt-3">
              <CopyField value={snippets.hosted} />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle>Keys</CardTitle>
          <CardDescription>The public key is safe in a browser. The secret key is for your server and the CLI.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-center">
          <CopyField value={p.public_key} />
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="outline">
                <KeyRoundIcon />
                Rotate secret key
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Rotate the secret key?</AlertDialogTitle>
                <AlertDialogDescription>The current key stops working immediately. Anything using it, like your backend or CLI, needs the new one.</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  onClick={async () => {
                    try {
                      const r = await api<{ secret_key: string }>(projectPath(slug, "/rotate"), { method: "POST" })
                      setSecret(r.secret_key)
                    } catch (err) {
                      toast.error((err as Error).message)
                    }
                  }}
                >
                  Rotate
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardContent>
      </Card>
      <SecretKeyDialog secret={secret} onClose={() => setSecret(null)} />
    </Page>
  )
}

function StatCard({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <Card className="gap-1 py-4">
      <CardHeader className="px-4">
        <CardDescription>{label}</CardDescription>
      </CardHeader>
      <CardContent className="px-4">
        <div className="text-2xl font-semibold tabular-nums tracking-tight">{value}</div>
        <div className="mt-0.5 truncate text-xs text-muted-foreground">{hint}</div>
      </CardContent>
    </Card>
  )
}

function RankCard({ title, rows, empty }: { title: string; empty: string; rows: { key: string; label: string; count: number }[] }) {
  const max = Math.max(1, ...rows.map((r) => r.count))
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">{empty}</p>
        ) : (
          <ul className="grid gap-1.5">
            {rows.map((r) => (
              <li key={r.key} className="relative flex items-center justify-between gap-3 rounded-md px-2 py-1 text-sm">
                <span className="absolute inset-y-0 left-0 rounded-md bg-[var(--chart-1)]/12" style={{ width: `${(r.count / max) * 100}%` }} />
                <span className="relative truncate">{r.label}</span>
                <span className="relative tabular-nums text-muted-foreground">{num(r.count)}</span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
