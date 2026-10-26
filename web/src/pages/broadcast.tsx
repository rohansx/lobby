import { SendIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { useParams } from "wouter"
import { Page } from "@/components/page"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { api, projectPath, type Broadcast, type Status } from "@/lib/api"
import { dateTime, num } from "@/lib/format"
import { useApi } from "@/lib/use-api"

const audiences = { confirmed: "Confirmed, not yet invited", invited: "Invited", all: "Everyone confirmed, invited or joined" }

export default function BroadcastPage() {
  const { slug } = useParams<{ slug: string }>()
  const { data, reload } = useApi<{ data: Broadcast[]; by_status: Partial<Record<Status, number>> }>(projectPath(slug, "/broadcasts"))
  const [subject, setSubject] = useState("")
  const [body, setBody] = useState("")
  const [audience, setAudience] = useState<keyof typeof audiences>("confirmed")
  const [busy, setBusy] = useState(false)

  const bs = data?.by_status ?? {}
  const reach = audience === "all" ? (bs.confirmed ?? 0) + (bs.invited ?? 0) + (bs.joined ?? 0) : (bs[audience] ?? 0)

  async function send() {
    setBusy(true)
    try {
      const r = await api<{ recipients: number }>(projectPath(slug, "/broadcasts"), { body: { subject, body, audience } })
      toast.success(`Queued for ${num(r.recipients)} people`)
      setSubject("")
      setBody("")
      reload()
    } catch (err) {
      toast.error((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Page title="Broadcast" description="One plain-text email to the list, for the launch. Every message has a one-click unsubscribe.">
      <Card>
        <CardContent className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="bc-audience">Audience</Label>
            <Select value={audience} onValueChange={(v) => setAudience(v as keyof typeof audiences)}>
              <SelectTrigger id="bc-audience" className="w-full sm:w-80">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(audiences).map(([k, v]) => (
                  <SelectItem key={k} value={k}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">About {num(reach)} people, minus anyone who unsubscribed.</p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="bc-subject">Subject</Label>
            <Input id="bc-subject" value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="We're live" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="bc-body">Message</Label>
            <Textarea id="bc-body" rows={10} value={body} onChange={(e) => setBody(e.target.value)} placeholder="Plain text. Links are fine." />
          </div>
          <div>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button disabled={busy || !subject.trim() || !body.trim() || reach === 0}>
                  <SendIcon />
                  Send to {num(reach)}
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Send “{subject}”?</AlertDialogTitle>
                  <AlertDialogDescription>It goes to about {num(reach)} people and can't be recalled.</AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={send}>Send</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </CardContent>
      </Card>

      {data && data.data.length > 0 && (
        <Card className="mt-6 overflow-hidden pb-0">
          <CardHeader>
            <CardTitle>Sent</CardTitle>
            <CardDescription>Past broadcasts for this project.</CardDescription>
          </CardHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-6">Subject</TableHead>
                <TableHead>Audience</TableHead>
                <TableHead className="text-right">Recipients</TableHead>
                <TableHead className="pr-6">Sent</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.data.map((b) => (
                <TableRow key={b.id}>
                  <TableCell className="max-w-72 truncate pl-6 font-medium">{b.subject}</TableCell>
                  <TableCell className="capitalize text-muted-foreground">{b.audience}</TableCell>
                  <TableCell className="text-right tabular-nums">{num(b.recipients)}</TableCell>
                  <TableCell className="pr-6 text-muted-foreground">{dateTime(b.created_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </Page>
  )
}
