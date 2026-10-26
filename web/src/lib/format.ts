const nf = new Intl.NumberFormat()
export const num = (n: number | undefined | null) => nf.format(n ?? 0)
export const pct = (f: number) => `${Math.round(f * 100)}%`

const dateFmt = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", year: "numeric" })
const dateTimeFmt = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })
export const date = (t: number) => dateFmt.format(new Date(t * 1000))
export const dateTime = (t: number) => dateTimeFmt.format(new Date(t * 1000))

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" })
export function ago(t: number) {
  const s = Math.round(t - Date.now() / 1000)
  const abs = Math.abs(s)
  if (abs < 60) return rtf.format(s, "second")
  if (abs < 3600) return rtf.format(Math.round(s / 60), "minute")
  if (abs < 86400) return rtf.format(Math.round(s / 3600), "hour")
  if (abs < 86400 * 30) return rtf.format(Math.round(s / 86400), "day")
  return date(t)
}
