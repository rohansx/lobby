import { LoaderIcon } from "lucide-react"
import { useEffect, useRef } from "react"
import { toast } from "sonner"
import { useLocation } from "wouter"
import { api, ApiError } from "@/lib/api"
import { PACK_PRICE } from "@/lib/pack"

export interface CheckoutIntent {
  name: string
}

// checkoutIntent says what a /checkout link from the pricing page buys. Every link
// leads to the Launch Pack now; old ?plan=launch|pro links included.
export function checkoutIntent(): CheckoutIntent {
  return { name: "Launch Pack" }
}

export function intentLine(i: CheckoutIntent): string {
  return `${i.name}, ${PACK_PRICE} one-time`
}

// Checkout is where the pricing page's buttons land once the visitor is
// signed in: it opens a Polar checkout and sends them straight there.
export default function Checkout() {
  const [, navigate] = useLocation()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    api<{ url: string }>("/billing/pack-checkout", { body: {} })
      .then((r) => window.location.replace(r.url))
      .catch((err) => {
        // Billing is down or not set up yet: the Launch page has the offer.
        toast.error(err instanceof ApiError ? err.message : "Checkout is unavailable right now.")
        navigate("/campaigns", { replace: true })
      })
  }, [navigate])

  return (
    <div className="flex min-h-[60svh] flex-col items-center justify-center gap-3 p-8 text-center">
      <LoaderIcon className="size-5 animate-spin text-muted-foreground" />
      <p className="text-sm text-muted-foreground">Taking you to checkout for {intentLine(checkoutIntent())}…</p>
    </div>
  )
}
