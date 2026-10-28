import { useEffect, useState } from "react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { useSession } from "@/lib/session"

// VerifyBanner nags a new cloud account to confirm its email, and thanks it
// when it lands back from the link.
export function VerifyBanner() {
  const { me } = useSession()
  const [sent, setSent] = useState(false)

  useEffect(() => {
    if (new URLSearchParams(window.location.search).has("verified")) {
      toast.success("Email confirmed. Create your first waitlist.")
      window.history.replaceState(null, "", window.location.pathname)
    }
  }, [])

  if (!me.cloud || me.verified) return null
  return (
    <div className="border-b bg-amber-500/10 px-4 py-2.5 text-sm md:px-8">
      Confirm your email to create waitlists. We sent a link to <span className="font-medium">{me.email}</span>.{" "}
      {sent ? (
        <span className="text-muted-foreground">Sent again.</span>
      ) : (
        <button
          type="button"
          className="font-medium underline underline-offset-2"
          onClick={async () => {
            try {
              await api("/verify/resend", { method: "POST" })
              setSent(true)
            } catch (err) {
              toastError(err)
            }
          }}
        >
          Send it again
        </button>
      )}
    </div>
  )
}
