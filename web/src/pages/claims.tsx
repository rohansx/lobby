import { BanIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Link, useParams } from "wouter"
import { CopyButton, CopyField } from "@/components/copy-field"
import { Page } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, projectPath, type Claim } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { ago, dateTime } from "@/lib/format"
import { useApi } from "@/lib/use-api"
import { cn } from "@/lib/utils"

interface Data {
  data: Claim[]
  target: string
}

const stateLabel: Record<Claim["state"], string> = {
  open: "Sent",
  opened: "Opened",
  verified: "Email confirmed",
  claimed: "Claimed",
  revoked: "Revoked",
  expired: "Expired",
}

export default function Claims() {
  const { slug } = useParams<{ slug: string }>()
  const { data, reload } = useApi<Data>(projectPath(slug, "/claims"))

  async function revoke(id: number) {
    try {
      await api(projectPath(slug, `/claims/${id}/revoke`), { method: "POST" })
      toast.success("Link revoked")
      reload()
    } catch (err) {
      toastError(err)
    }
  }

  return (
    <Page
      title="Claims"
      description="Build an account in your product for a prospect, then send a link. Lobby confirms their email and your app hands the account over."
      actions={data?.target && <NewClaim slug={slug} onCreated={reload} />}
    >
      {!data ? (
        <Skeleton className="h-40" />
      ) : !data.target ? (
        <Card className="border-dashed">
          <CardHeader className="text-center">
            <CardTitle>Set an Invite URL first</CardTitle>
            <CardDescription>
              After confirming their email, people land on your waitlist's Invite URL with <code>?claim=…</code>. Your app redeems that code for the account. Set it in{" "}
              <Link className="underline" href={`/p/${slug}/settings`}>
                Settings
              </Link>
              .
            </CardDescription>
          </CardHeader>
        </Card>
      ) : data.data.length === 0 ? (
        <Card className="border-dashed">
          <CardHeader className="text-center">
            <CardTitle>No claims yet</CardTitle>
            <CardDescription>
              Create one with your product's id for the account. People land on <span className="font-mono">{data.target}</span> with <code>?claim=…</code>, and your app calls{" "}
              <code>POST /v1/claims/redeem</code>. See <span className="font-mono">/llms.txt</span> for the details.
            </CardDescription>
          </CardHeader>
        </Card>
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">Account</TableHead>
                <TableHead>State</TableHead>
                <TableHead className="hidden md:table-cell">Email</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.data.map((c) => {
                const live = ["open", "opened", "verified"].includes(c.state)
                return (
                  <TableRow key={c.id}>
                    <TableCell className="pl-4">
                      <div className="font-medium">{c.label || c.account_ref}</div>
                      {c.label && <div className="font-mono text-xs text-muted-foreground">{c.account_ref}</div>}
                    </TableCell>
                    <TableCell>
                      <Badge variant={c.state === "claimed" ? "default" : "outline"} className={cn(!live && c.state !== "claimed" && "text-muted-foreground")}>
                        {stateLabel[c.state]}
                      </Badge>
                    </TableCell>
                    <TableCell className="hidden text-sm md:table-cell">
                      {c.email || (c.pending_email ? <span className="text-muted-foreground">{c.pending_email}, not confirmed</span> : c.email_lock ? <span className="text-muted-foreground">only {c.email_lock}</span> : <span className="text-muted-foreground">–</span>)}
                    </TableCell>
                    <TableCell className="text-muted-foreground" title={dateTime(c.created_at)}>
                      {ago(c.created_at)}
                    </TableCell>
                    <TableCell className="pr-2 text-right">
                      {live && (
                        <span className="inline-flex items-center gap-1">
                          <CopyButton value={c.claim_url} />
                          <Button variant="ghost" size="icon-sm" aria-label="Revoke" title="Revoke" onClick={() => revoke(c.id)}>
                            <BanIcon />
                          </Button>
                        </span>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}
    </Page>
  )
}

function NewClaim({ slug, onCreated }: { slug: string; onCreated: () => void }) {
  const [open, setOpen] = useState(false)
  const [ref, setRef] = useState("")
  const [label, setLabel] = useState("")
  const [email, setEmail] = useState("")
  const [link, setLink] = useState("")
  const [error, setError] = useState("")

  function reset(o: boolean) {
    setOpen(o)
    if (!o) {
      setRef("")
      setLabel("")
      setEmail("")
      setLink("")
      setError("")
    }
  }

  return (
    <Dialog open={open} onOpenChange={reset}>
      <DialogTrigger asChild>
        <Button>
          <PlusIcon />
          New claim
        </Button>
      </DialogTrigger>
      <DialogContent>
        {link ? (
          <div className="grid gap-4">
            <DialogHeader>
              <DialogTitle>Send this link</DialogTitle>
              <DialogDescription>Email it, DM it or put it on a slide. It works for 30 days, and only once.</DialogDescription>
            </DialogHeader>
            <CopyField value={link} multiline />
            <DialogFooter>
              <Button onClick={() => reset(false)}>Done</Button>
            </DialogFooter>
          </div>
        ) : (
          <form
            className="grid gap-4"
            onSubmit={async (e) => {
              e.preventDefault()
              setError("")
              try {
                const c = await api<{ claim_url: string }>(projectPath(slug, "/claims"), { body: { account_ref: ref, label, email } })
                setLink(c.claim_url)
                onCreated()
              } catch (err) {
                setError((err as Error).message)
              }
            }}
          >
            <DialogHeader>
              <DialogTitle>New claim</DialogTitle>
              <DialogDescription>For an account you've already set up in your product. A new claim for the same account replaces the old link.</DialogDescription>
            </DialogHeader>
            <div className="grid gap-2">
              <Label htmlFor="cl-ref">Account id in your product</Label>
              <Input id="cl-ref" required maxLength={200} placeholder="acct_123" value={ref} onChange={(e) => setRef(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="cl-label">Who it's for (optional)</Label>
              <Input id="cl-label" maxLength={120} placeholder="Acme, Priya" value={label} onChange={(e) => setLabel(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="cl-email">Only this email can claim it (optional)</Label>
              <Input id="cl-email" type="email" placeholder="priya@acme.com" value={email} onChange={(e) => setEmail(e.target.value)} />
              <p className="text-xs text-muted-foreground">Leave empty to let whoever has the link claim it with their own address. Either way, the email is confirmed first.</p>
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <DialogFooter>
              <Button type="submit">Create link</Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
