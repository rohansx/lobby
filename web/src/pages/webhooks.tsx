import { PlusIcon, RotateCwIcon, Trash2Icon, ZapIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { useParams } from "wouter"
import { CopyButton } from "@/components/copy-field"
import { Page } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, projectPath, type Delivery, type Webhook } from "@/lib/api"
import { ago, dateTime } from "@/lib/format"
import { useApi } from "@/lib/use-api"
import { cn } from "@/lib/utils"

interface Data {
  webhooks: Webhook[]
  deliveries: Delivery[]
  events: string[]
}

export default function Webhooks() {
  const { slug } = useParams<{ slug: string }>()
  const { data, reload } = useApi<Data>(projectPath(slug, "/webhooks"))

  async function act(path: string, ok: string) {
    try {
      await api(projectPath(slug, path), { method: "POST" })
      toast.success(ok)
      reload()
    } catch (err) {
      toast.error((err as Error).message)
    }
  }

  return (
    <Page
      title="Webhooks"
      description="Signed with the Standard Webhooks scheme, retried for about a day. A 410 response disables the endpoint."
      actions={data && <AddWebhook slug={slug} events={data.events} onAdded={reload} />}
    >
      {!data ? (
        <Skeleton className="h-40" />
      ) : data.webhooks.length === 0 ? (
        <Card className="border-dashed">
          <CardHeader className="text-center">
            <CardTitle>No endpoints</CardTitle>
            <CardDescription>Add one to hear about signups, confirmations, referrals, invites and joins. Slack incoming webhooks work too.</CardDescription>
          </CardHeader>
        </Card>
      ) : (
        <div className="grid gap-3">
          {data.webhooks.map((w) => (
            <Card key={w.id} className="py-4">
              <CardContent className="flex flex-wrap items-center gap-3 px-4">
                <Switch
                  checked={w.active}
                  onCheckedChange={(c) => act(`/webhooks/${w.id}/${c ? "enable" : "disable"}`, c ? "Enabled" : "Disabled")}
                  aria-label="Active"
                />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-sm">{w.url}</div>
                  <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                    <Badge variant="outline">{w.kind === "slack" ? "Slack" : "Standard"}</Badge>
                    {w.events === "*" ? <span>All events</span> : w.events.split(",").map((e) => <Badge key={e} variant="secondary" className="font-mono">{e}</Badge>)}
                    {w.disabled_reason && <span className="text-destructive">{w.disabled_reason}</span>}
                  </div>
                </div>
                {w.kind === "standard" && w.secret && (
                  <div className="flex items-center gap-1 rounded-md border bg-muted/40 pl-2 text-xs">
                    <span className="font-mono text-muted-foreground">{w.secret.slice(0, 12)}…</span>
                    <CopyButton value={w.secret} />
                  </div>
                )}
                <Button variant="outline" size="sm" onClick={() => act(`/webhooks/${w.id}/test`, "Test event queued")}>
                  <ZapIcon />
                  Send test
                </Button>
                <Button variant="ghost" size="icon-sm" aria-label="Delete" onClick={() => act(`/webhooks/${w.id}/delete`, "Deleted")}>
                  <Trash2Icon />
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <h2 className="mt-8 mb-3 text-sm font-medium">Recent deliveries</h2>
      <Card className="overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-4">Event</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Attempts</TableHead>
              <TableHead className="hidden md:table-cell">Last error</TableHead>
              <TableHead>When</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {data?.deliveries.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="py-10 text-center text-muted-foreground">
                  Nothing sent yet.
                </TableCell>
              </TableRow>
            )}
            {data?.deliveries.map((d) => (
              <TableRow key={d.id}>
                <TableCell className="pl-4 font-mono text-xs">{d.event}</TableCell>
                <TableCell>
                  <span className={cn("inline-flex items-center gap-1.5 text-sm", d.status === "failed" && "text-destructive")}>
                    <span className={cn("size-1.5 rounded-full", d.status === "delivered" ? "bg-emerald-500" : d.status === "failed" ? "bg-destructive" : "bg-amber-500")} />
                    {d.status}
                    {d.response_code > 0 && <span className="text-muted-foreground tabular-nums">{d.response_code}</span>}
                  </span>
                </TableCell>
                <TableCell className="text-right tabular-nums">{d.attempts}</TableCell>
                <TableCell className="hidden max-w-64 truncate text-xs text-muted-foreground md:table-cell">{d.last_error || "–"}</TableCell>
                <TableCell className="text-muted-foreground" title={dateTime(d.created_at)}>
                  {ago(d.created_at)}
                </TableCell>
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="icon-sm" aria-label="Redeliver" onClick={() => act(`/deliveries/${d.id}/redeliver`, "Redelivery queued")}>
                        <RotateCwIcon />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>Redeliver</TooltipContent>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </Page>
  )
}

function AddWebhook({ slug, events, onAdded }: { slug: string; events: string[]; onAdded: () => void }) {
  const [open, setOpen] = useState(false)
  const [url, setUrl] = useState("")
  const [kind, setKind] = useState("standard")
  const [chosen, setChosen] = useState<string[]>([])
  const [error, setError] = useState("")

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <PlusIcon />
          Add endpoint
        </Button>
      </DialogTrigger>
      <DialogContent>
        <form
          className="grid gap-4"
          onSubmit={async (e) => {
            e.preventDefault()
            setError("")
            try {
              await api(projectPath(slug, "/webhooks"), { body: { url, kind, events: chosen } })
              toast.success("Endpoint added")
              setOpen(false)
              setUrl("")
              setChosen([])
              onAdded()
            } catch (err) {
              setError((err as Error).message)
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>Add endpoint</DialogTitle>
            <DialogDescription>Events are sent as JSON POSTs with webhook-id, webhook-timestamp and webhook-signature headers.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="wh-url">URL</Label>
            <Input id="wh-url" type="url" required placeholder="https://example.com/hooks/lobby" value={url} onChange={(e) => setUrl(e.target.value)} />
          </div>
          <div className="grid gap-2">
            <Label>Format</Label>
            <Select value={kind} onValueChange={setKind}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="standard">Standard Webhooks (signed JSON)</SelectItem>
                <SelectItem value="slack">Slack incoming webhook</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <fieldset className="grid gap-2">
            <legend className="mb-2 text-sm font-medium">Events</legend>
            <div className="grid grid-cols-2 gap-2">
              {events.map((ev) => (
                <label key={ev} className="flex items-center gap-2 font-mono text-xs">
                  <Checkbox checked={chosen.includes(ev)} onCheckedChange={(c) => setChosen((cur) => (c ? [...cur, ev] : cur.filter((x) => x !== ev)))} />
                  {ev}
                </label>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">Leave all unchecked to receive every event.</p>
          </fieldset>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <Button type="submit">Add endpoint</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
