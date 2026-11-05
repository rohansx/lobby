import { useState } from "react"
import { toast } from "sonner"
import { api, ApiError } from "@/lib/api"
import { toastError } from "@/lib/errors"

// ponytail: pilot price shown in the dashboard; Polar's product sets what is actually charged.
export const PACK_PRICE = "$99"

// useBuyPack opens a Polar checkout for one Launch Pack.
export function useBuyPack() {
  const [busy, setBusy] = useState(false)
  const buy = async () => {
    setBusy(true)
    try {
      const r = await api<{ url: string }>("/billing/pack-checkout", { body: {} })
      window.location.href = r.url
    } catch (err) {
      setBusy(false)
      if (err instanceof ApiError && (err.status === 404 || err.status === 405 || err.status === 503)) {
        toast.error("Checkout for the Launch Pack isn't open yet. Try again soon.")
      } else {
        toastError(err)
      }
    }
  }
  return { busy, buy }
}
