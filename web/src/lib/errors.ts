import { toast } from "sonner"
import { navigate } from "wouter/use-browser-location"
import { ApiError } from "./api"

// toastError shows an API error; a plan limit gets a button to the plans.
export function toastError(err: unknown) {
  const msg = (err as Error).message
  if (err instanceof ApiError && err.upgrade) {
    toast.error(msg, { action: { label: "See plans", onClick: () => navigate("/billing") } })
  } else {
    toast.error(msg)
  }
}
