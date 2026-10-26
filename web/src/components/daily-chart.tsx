import { useMemo, useState } from "react"
import { num } from "@/lib/format"

// 30 days of signups: one series, thin bars with a 2px gap and rounded tops,
// a recessive baseline and a hover tooltip. No legend; the card title names it.
const W = 600
const H = 160
const GAP = 2
const DAYS = 30

function niceCeil(n: number) {
  if (n <= 0) return 0
  for (let mag = 1; ; mag *= 10) for (const m of [1, 2, 5]) if (m * mag >= n) return m * mag
}

const dayFmt = new Intl.DateTimeFormat(undefined, { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" })

export function DailyChart({ daily }: { daily: { day: string; count: number }[] }) {
  const [hover, setHover] = useState<number | null>(null)
  const { bars, top } = useMemo(() => {
    const counts = new Map(daily.map((d) => [d.day, d.count]))
    const start = Date.now() - (DAYS - 1) * 86400000
    const days = Array.from({ length: DAYS }, (_, i) => {
      const day = new Date(start + i * 86400000).toISOString().slice(0, 10)
      return { day, count: counts.get(day) ?? 0 }
    })
    return { bars: days, top: niceCeil(Math.max(0, ...days.map((d) => d.count))) }
  }, [daily])

  const bw = W / DAYS - GAP
  const h = (c: number) => (top ? Math.max(c > 0 ? 2 : 0, (c / top) * H) : 0)
  const active = hover === null ? null : bars[hover]

  return (
    <div className="relative">
      <div className="flex gap-2">
        <div className="flex h-[160px] flex-col justify-between py-0 text-right text-[11px] tabular-nums text-muted-foreground">
          <span>{num(top)}</span>
          <span>0</span>
        </div>
        <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="h-[160px] w-full overflow-visible" role="img" aria-label="Signups per day, last 30 days" onMouseLeave={() => setHover(null)}>
          <line x1={0} x2={W} y1={H - 0.5} y2={H - 0.5} className="stroke-border" strokeWidth={1} vectorEffect="non-scaling-stroke" />
          {bars.map((b, i) => {
            const bh = h(b.count)
            const x = i * (bw + GAP)
            const r = Math.min(4, bw / 2, bh)
            return (
              <g key={b.day} onMouseEnter={() => setHover(i)}>
                <rect x={x - GAP / 2} y={0} width={bw + GAP} height={H} fill="transparent" />
                {bh > 0 && (
                  <path
                    d={`M${x},${H} V${H - bh + r} Q${x},${H - bh} ${x + r},${H - bh} H${x + bw - r} Q${x + bw},${H - bh} ${x + bw},${H - bh + r} V${H} Z`}
                    fill="var(--chart-1)"
                    opacity={hover === null || hover === i ? 1 : 0.45}
                  />
                )}
              </g>
            )
          })}
        </svg>
      </div>
      <div className="mt-1.5 flex justify-between pl-8 text-[11px] text-muted-foreground">
        <span>{dayFmt.format(new Date(bars[0].day))}</span>
        <span>Today</span>
      </div>
      {active && hover !== null && (
        <div
          className="pointer-events-none absolute top-0 z-10 -translate-x-1/2 rounded-md border bg-popover px-2.5 py-1.5 text-xs shadow-md"
          style={{ left: `calc(2rem + ${((hover + 0.5) / DAYS) * 100}% - ${((hover + 0.5) / DAYS) * 2}rem)` }}
        >
          <div className="text-muted-foreground">{dayFmt.format(new Date(active.day))}</div>
          <div className="font-medium tabular-nums">{num(active.count)} signups</div>
        </div>
      )}
    </div>
  )
}
