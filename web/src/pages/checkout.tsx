import { LoaderIcon } from "lucide-react"
import { useEffect, useRef } from "react"
import { toast } from "sonner"
import { useLocation } from "wouter"
import { api, ApiError } from "@/lib/api"

// The plans the pricing page links to, for the line shown on the way to checkout.
export const paidPlans: Record<string, { name: string; month: number; year: number }> = {
  launch: { name: "Launch", month: 5, year: 4 },
  pro: { name: "Pro", month: 12, year: 10 },
}

// checkoutIntent reads plan and interval off a /checkout?plan=…&interval=… URL.
export function checkoutIntent(search: string) {
  const q = new URLSearchParams(search)
  const plan = q.get("plan") ?? ""
  const interval = q.get("interval") === "year" ? "year" : "month"
  return paidPlans[plan] ? { plan, interval, ...paidPlans[plan] } : null
}

export function intentLine(i: NonNullable<ReturnType<typeof checkoutIntent>>) {
  return i.interval === "year" ? `${i.name}, $${i.year}/mo billed yearly ($${i.year * 12})` : `${i.name}, $${i.month}/mo`
}

// Checkout is where the pricing page's buttons land once the visitor is
// signed in: it opens a Polar checkout and sends them straight there.
export default function Checkout() {
  const [, navigate] = useLocation()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    const intent = checkoutIntent(window.location.search)
    if (!intent) {
      navigate("/billing", { replace: true })
      return
    }
    api<{ url: string }>("/billing/checkout", { body: { plan: intent.plan, interval: intent.interval } })
      .then((r) => window.location.replace(r.url))
      .catch((err) => {
        // Already subscribed, or billing is down: the billing page explains.
        toast.error(err instanceof ApiError ? err.message : "Checkout is unavailable right now.")
        navigate("/billing", { replace: true })
      })
  }, [navigate])

  const intent = checkoutIntent(window.location.search)
  return (
    <div className="flex min-h-[60svh] flex-col items-center justify-center gap-3 p-8 text-center">
      <LoaderIcon className="size-5 animate-spin text-muted-foreground" />
      <p className="text-sm text-muted-foreground">Taking you to checkout{intent ? ` for ${intentLine(intent)}` : ""}…</p>
    </div>
  )
}
