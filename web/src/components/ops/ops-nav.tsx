import { Link, useLocation } from "wouter"
import { Button } from "@/components/ui/button"

const sections = [
  { href: "/ops", label: "Accounts" },
  { href: "/ops/campaigns", label: "Campaigns" },
  { href: "/ops/destinations", label: "Destinations" },
]

// Navigation between the operator pages.
export function OpsNav() {
  const [location] = useLocation()
  const current = [...sections].reverse().find((s) => location === s.href || location.startsWith(s.href + "/"))
  return (
    <nav className="mb-6 flex flex-wrap gap-1" aria-label="Operator sections">
      {sections.map((s) => (
        <Button key={s.href} asChild size="sm" variant={current?.href === s.href ? "secondary" : "ghost"}>
          <Link href={s.href} aria-current={current?.href === s.href ? "page" : undefined}>
            {s.label}
          </Link>
        </Button>
      ))}
    </nav>
  )
}
