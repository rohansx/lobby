import { ExternalLinkIcon, PlusIcon } from "lucide-react"
import { toast } from "sonner"
import { DestinationDialog } from "@/components/ops/destination-dialog"
import { OpsNav } from "@/components/ops/ops-nav"
import { Page } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, type Destination } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { date, money } from "@/lib/format"
import { useApi } from "@/lib/use-api"

// The operator's catalogue of launch destinations. Nothing is seeded; each entry is researched and verified by hand.
export default function OpsDestinations() {
  const { data, reload } = useApi<{ data: Destination[] }>("/ops/destinations")

  async function setActive(d: Destination, active: boolean) {
    try {
      await api(`/ops/destinations/${d.id}`, { method: "PATCH", body: { active } })
      toast.success(`${d.name} is ${active ? "active" : "inactive"}`)
      reload()
    } catch (err) {
      toastError(err)
    }
  }

  return (
    <Page
      title="Operator"
      description="Destinations you can put on a launch plan."
      actions={
        <DestinationDialog onSaved={reload}>
          <Button>
            <PlusIcon />
            New destination
          </Button>
        </DestinationDialog>
      }
    >
      <OpsNav />
      {!data ? (
        <Skeleton className="h-40" />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-4">Destination</TableHead>
                <TableHead>Stages</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead className="text-right">Fee</TableHead>
                <TableHead>Verified</TableHead>
                <TableHead className="w-40" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.data.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="py-10 text-center text-muted-foreground">
                    No destinations yet. Add the ones you've checked accept submissions.
                  </TableCell>
                </TableRow>
              )}
              {data.data.map((d) => (
                <TableRow key={d.id} className={d.active ? undefined : "text-muted-foreground"}>
                  <TableCell className="max-w-72 pl-4">
                    <div className="flex items-center gap-1.5">
                      <a href={d.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 truncate font-medium hover:underline">
                        {d.name}
                        <ExternalLinkIcon className="size-3 shrink-0 text-muted-foreground" />
                      </a>
                      {!d.active && <Badge variant="outline">inactive</Badge>}
                    </div>
                    <div className="text-xs text-muted-foreground">{d.slug}</div>
                  </TableCell>
                  <TableCell className="capitalize">{d.stages ? d.stages.replaceAll(",", ", ") : "any"}</TableCell>
                  <TableCell className="capitalize">{d.mode}</TableCell>
                  <TableCell className="text-right tabular-nums">{d.fee_cents > 0 ? money(d.fee_cents) : "Free"}</TableCell>
                  <TableCell>{d.last_verified_at ? date(d.last_verified_at) : <span className="text-amber-700 dark:text-amber-300">never</span>}</TableCell>
                  <TableCell className="pr-4 text-right whitespace-nowrap">
                    <DestinationDialog destination={d} onSaved={reload}>
                      <Button variant="ghost" size="sm">
                        Edit
                      </Button>
                    </DestinationDialog>
                    <Button variant="ghost" size="sm" onClick={() => setActive(d, !d.active)}>
                      {d.active ? "Deactivate" : "Activate"}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </Page>
  )
}
