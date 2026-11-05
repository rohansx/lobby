import { CheckIcon, LoaderIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { PACK_PRICE, useBuyPack } from "@/lib/pack"

const included = [
  "A readiness review of your landing page, demo, signup flow and assets, with a short list of blockers",
  "One product profile and asset kit, reused for every destination",
  "A shortlist of up to 20 destinations that suit your product and stage, which you approve first",
  "Submissions done for you where that's allowed, and clear instructions where you post yourself",
  "Submission work done within 10 business days of your plan approval, then 30 days of follow-up tracking",
  "A results report tied to your waitlist signups: visits, signups, confirmations and what's still pending",
]

const notPromised = "Acceptance, rankings, upvotes, backlinks, traffic or customers aren't promised. Each platform decides if and when it publishes. Platform fees, where a destination charges one, are listed separately and approved by you. If a destination closes or turns the product away, we record why and offer a suitable replacement where there is one."

export function PackOffer() {
  const { busy, buy } = useBuyPack()
  return (
    <Card className="max-w-2xl">
      <CardHeader>
        <CardTitle>Launch Pack</CardTitle>
        <CardDescription>
          One payment, no subscription. <span className="text-foreground tabular-nums">{PACK_PRICE}</span> pilot price.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 text-sm">
        <ul className="grid gap-2">
          {included.map((f) => (
            <li key={f} className="flex items-start gap-2">
              <CheckIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              {f}
            </li>
          ))}
        </ul>
        <p className="text-muted-foreground">{notPromised}</p>
      </CardContent>
      <CardFooter className="border-t">
        <Button disabled={busy} onClick={buy}>
          {busy && <LoaderIcon className="animate-spin" />}
          Buy the Launch Pack
        </Button>
      </CardFooter>
    </Card>
  )
}
