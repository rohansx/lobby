import { ExternalLinkIcon, MoreHorizontalIcon, SearchIcon } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { toast } from "sonner"
import { useParams } from "wouter"
import { Page } from "@/components/page"
import { StatusBadge } from "@/components/status-badge"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, projectPath, STATUSES, type Signup } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { ago, dateTime, num } from "@/lib/format"
import { useSession } from "@/lib/session"
import { invalidate, useApi } from "@/lib/use-api"

type Action = "invite" | "confirm" | "resend" | "joined" | "remove" | "delete"

const actionLabels: Record<Action, string> = {
  invite: "Invite",
  confirm: "Mark confirmed",
  resend: "Resend status email",
  joined: "Mark joined",
  remove: "Remove",
  delete: "Delete and suppress",
}

export default function Signups() {
  const { slug } = useParams<{ slug: string }>()
  const { refreshProjects } = useSession()
  const [status, setStatus] = useState("all")
  const [search, setSearch] = useState("")
  const [q, setQ] = useState("")
  const [pages, setPages] = useState<Signup[][]>([])
  const [next, setNext] = useState(0)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [confirming, setConfirming] = useState<{ action: Action; ids: number[] } | null>(null)

  useEffect(() => {
    const t = setTimeout(() => setQ(search.trim()), 250)
    return () => clearTimeout(t)
  }, [search])

  const params = new URLSearchParams()
  if (status !== "all") params.set("status", status)
  if (q) params.set("q", q)
  const path = projectPath(slug, `/signups?${params}`)
  const { data, loading, reload } = useApi<{ data: Signup[]; next: number }>(path)

  useEffect(() => {
    if (data) {
      setPages([data.data])
      setNext(data.next)
    }
  }, [data])

  useEffect(() => setSelected(new Set()), [path])

  const rows = useMemo(() => pages.flat(), [pages])
  const allChecked = rows.length > 0 && rows.every((r) => selected.has(r.id))

  async function loadMore() {
    const p = new URLSearchParams(params)
    p.set("before", String(next))
    const r = await api<{ data: Signup[]; next: number }>(projectPath(slug, `/signups?${p}`))
    setPages((ps) => [...ps, r.data])
    setNext(r.next)
  }

  async function run(action: Action, ids: number[]) {
    try {
      const r = await api<{ message: string }>(projectPath(slug, "/signups/action"), { body: { action, ids } })
      toast.success(r.message)
      setSelected(new Set())
      invalidate(projectPath(slug))
      reload()
      refreshProjects()
    } catch (err) {
      toastError(err)
    }
  }

  const request = (action: Action, ids: number[]) => (action === "remove" || action === "delete" ? setConfirming({ action, ids }) : run(action, ids))

  return (
    <Page title="Signups" description="Newest first. Place in line is shown for confirmed people.">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1 sm:max-w-xs">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input placeholder="Search email" value={search} onChange={(e) => setSearch(e.target.value)} className="pl-8" />
        </div>
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            {STATUSES.map((s) => (
              <SelectItem key={s} value={s} className="capitalize">
                {s}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {selected.size > 0 && (
          <div className="ml-auto flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{num(selected.size)} selected</span>
            <Button size="sm" onClick={() => request("invite", [...selected])}>
              Invite
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="sm" variant="outline">
                  More
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <ActionItems onSelect={(a) => request(a, [...selected])} />
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )}
      </div>

      <Card className="overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10 pl-4">
                <Checkbox
                  checked={allChecked}
                  onCheckedChange={(c) => setSelected(c ? new Set(rows.map((r) => r.id)) : new Set())}
                  aria-label="Select all"
                />
              </TableHead>
              <TableHead>Email</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Place</TableHead>
              <TableHead className="hidden text-right md:table-cell">Referrals</TableHead>
              <TableHead className="hidden lg:table-cell">Source</TableHead>
              <TableHead className="hidden sm:table-cell">Signed up</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {!data &&
              [0, 1, 2, 3, 4].map((i) => (
                <TableRow key={i}>
                  <TableCell colSpan={8} className="pl-4">
                    <Skeleton className="h-5" />
                  </TableCell>
                </TableRow>
              ))}
            {data && rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={8} className="py-12 text-center text-muted-foreground">
                  {q || status !== "all" ? "No signups match." : "No signups yet."}
                </TableCell>
              </TableRow>
            )}
            {rows.map((s) => (
              <TableRow key={s.id} data-state={selected.has(s.id) ? "selected" : undefined} className={loading ? "opacity-70" : undefined}>
                <TableCell className="pl-4">
                  <Checkbox
                    checked={selected.has(s.id)}
                    onCheckedChange={(c) =>
                      setSelected((cur) => {
                        const n = new Set(cur)
                        if (c) n.add(s.id)
                        else n.delete(s.id)
                        return n
                      })
                    }
                    aria-label={`Select ${s.email}`}
                  />
                </TableCell>
                <TableCell className="max-w-64 truncate font-medium">
                  {s.email}
                  {s.unsubscribed_at && <span className="ml-2 text-xs font-normal text-muted-foreground">unsubscribed</span>}
                </TableCell>
                <TableCell>
                  <StatusBadge status={s.status} />
                </TableCell>
                <TableCell className="text-right tabular-nums">{s.position ? `#${num(s.position)}` : <span className="text-muted-foreground">–</span>}</TableCell>
                <TableCell className="hidden text-right tabular-nums md:table-cell">{s.referrals || <span className="text-muted-foreground">0</span>}</TableCell>
                <TableCell className="hidden max-w-40 truncate text-muted-foreground lg:table-cell">{s.utm_source || (s.referred_by ? "referral" : host(s.referrer)) || "–"}</TableCell>
                <TableCell className="hidden text-muted-foreground sm:table-cell" title={dateTime(s.created_at)}>
                  {ago(s.created_at)}
                </TableCell>
                <TableCell>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-sm" aria-label="Actions">
                        <MoreHorizontalIcon />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem asChild>
                        <a href={s.status_url} target="_blank" rel="noreferrer">
                          <ExternalLinkIcon />
                          Open status page
                        </a>
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <ActionItems onSelect={(a) => request(a, [s.id])} />
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
      {next > 0 && (
        <div className="mt-4 flex justify-center">
          <Button variant="outline" onClick={loadMore}>
            Load more
          </Button>
        </div>
      )}

      <AlertDialog open={!!confirming} onOpenChange={(o) => !o && setConfirming(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirming?.action === "delete" ? "Delete" : "Remove"} {num(confirming?.ids.length)} {confirming?.ids.length === 1 ? "person" : "people"}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirming?.action === "delete"
                ? "Their data is erased for good and the address is suppressed so it can't sign up again. Use this for deletion requests."
                : "They leave the line and stop getting email. The record stays, so you can see they were here."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={() => confirming && run(confirming.action, confirming.ids)}>
              {confirming?.action === "delete" ? "Delete" : "Remove"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Page>
  )
}

function ActionItems({ onSelect }: { onSelect: (a: Action) => void }) {
  return (
    <>
      {(["invite", "confirm", "resend", "joined"] as Action[]).map((a) => (
        <DropdownMenuItem key={a} onSelect={() => onSelect(a)}>
          {actionLabels[a]}
        </DropdownMenuItem>
      ))}
      <DropdownMenuSeparator />
      {(["remove", "delete"] as Action[]).map((a) => (
        <DropdownMenuItem key={a} variant="destructive" onSelect={() => onSelect(a)}>
          {actionLabels[a]}
        </DropdownMenuItem>
      ))}
    </>
  )
}

function host(u: string) {
  try {
    return u ? new URL(u).host : ""
  } catch {
    return ""
  }
}
