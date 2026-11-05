import { LoaderIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { toast } from "sonner"
import { Link, useLocation } from "wouter"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { CampaignBadge } from "@/components/status-badge"
import { PackOffer } from "@/components/pack-offer"
import { Skeleton } from "@/components/ui/skeleton"
import { type Campaign } from "@/lib/api"
import { date } from "@/lib/format"
import { useSession } from "@/lib/session"
import { useApi } from "@/lib/use-api"

export default function Campaigns() {
  const { data, error, reload } = useApi<{ data: Campaign[] }>("/campaigns")
  const activating = useAfterCheckout(data?.data, reload)
  if (error) return <LoadError error={error} />
  return (
    <Page title="Launch" description="Your Launch Pack campaigns: one product, one plan, every submission tracked.">
      {activating && (
        <div className="mb-6 flex items-center gap-2 rounded-lg border bg-muted/50 px-4 py-3 text-sm">
          <LoaderIcon className="size-4 animate-spin" />
          Payment received. Opening your campaign…
        </div>
      )}
      {!data ? (
        <Skeleton className="h-48" />
      ) : data.data.length === 0 ? (
        <PackOffer />
      ) : (
        <div className="grid gap-3">
          {data.data.map((c) => (
            <Link key={c.id} href={`/campaigns/${c.id}`} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 hover:bg-muted/50">
              <div className="min-w-0">
                <div className="font-medium">
                  Campaign #{c.id}
                  {c.project_slug && <span className="text-muted-foreground"> · {c.project_slug}</span>}
                </div>
                <div className="text-xs text-muted-foreground">
                  Started {date(c.created_at)} · {c.tasks} {c.tasks === 1 ? "destination" : "destinations"}
                </div>
              </div>
              <CampaignBadge status={c.status} />
            </Link>
          ))}
        </div>
      )}
    </Page>
  )
}

// useAfterCheckout: back from Polar (?checkout=…), the campaign appears when
// the order webhook lands, usually within seconds, so poll briefly until a
// campaign made in the last few minutes shows up.
function useAfterCheckout(list: Campaign[] | undefined, reload: () => Promise<unknown>) {
  const [, navigate] = useLocation()
  const { refreshMe } = useSession()
  const [activating, setActivating] = useState(() => new URLSearchParams(window.location.search).has("checkout"))
  const [since] = useState(() => Date.now() / 1000 - 600)
  const arrived = !!list?.some((c) => c.created_at > since)
  useEffect(() => {
    if (!activating) return
    const finish = (msg: string) => {
      setActivating(false)
      navigate("/campaigns", { replace: true })
      toast(msg)
    }
    if (arrived) {
      refreshMe()
      finish("Your Launch Pack is ready. Start with your product profile.")
      return
    }
    let tries = 0
    const t = setInterval(() => {
      if (++tries >= 15) {
        clearInterval(t)
        finish("Payment received. Your campaign will appear here in a minute or two.")
      } else reload()
    }, 2000)
    return () => clearInterval(t)
  }, [activating, arrived, navigate, reload, refreshMe])
  return activating
}

