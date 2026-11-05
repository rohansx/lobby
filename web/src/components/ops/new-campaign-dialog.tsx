import { useState, type ReactNode } from "react"
import { toast } from "sonner"
import { useLocation } from "wouter"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, type Campaign, type OpsAccount } from "@/lib/api"
import { invalidate } from "@/lib/use-api"
import { badCents, toCents } from "./units"

// Opens a Launch Pack campaign by hand, e.g. for a pack paid outside Polar.
export function NewCampaignDialog({ account, children }: { account: OpsAccount; children: ReactNode }) {
  const [, navigate] = useLocation()
  const [open, setOpen] = useState(false)
  const [price, setPrice] = useState("99")
  const [order, setOrder] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const cents = toCents(price)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError("")
    try {
      const c = await api<Campaign>("/ops/campaigns", { body: { account_id: account.id, price_cents: cents, polar_order_id: order.trim() } })
      invalidate("/ops")
      toast.success(`Campaign #${c.id} opened for ${account.email}`)
      setOpen(false)
      navigate(`/ops/campaigns/${c.id}`)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent>
        <form className="grid gap-4" onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>New campaign for {account.email}</DialogTitle>
            <DialogDescription>Opens a Launch Pack in intake, puts the account on the pack plan and emails the founder.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="nc-price">Price paid (USD)</Label>
            <Input id="nc-price" inputMode="decimal" value={price} onChange={(e) => setPrice(e.target.value)} aria-invalid={badCents(cents)} />
            <p className="text-xs text-muted-foreground">0 for a free pilot.</p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="nc-order">Order reference (optional)</Label>
            <Input id="nc-order" maxLength={200} value={order} onChange={(e) => setOrder(e.target.value)} />
            <p className="text-xs text-muted-foreground">With a Polar order id, the same order can't open a second campaign.</p>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <Button type="submit" disabled={busy || badCents(cents)}>
              Open campaign
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
