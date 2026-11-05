// Dollars typed by an operator, as cents; NaN when it isn't a number.
export const toCents = (dollars: string) => (dollars.trim() === "" ? 0 : Math.round(Number(dollars) * 100))
export const toDollars = (cents: number) => (cents / 100).toFixed(2)
export const badCents = (cents: number) => Number.isNaN(cents) || cents < 0 || cents > 10_000_000

// <input type="date"> values are local calendar days; the API stores unix seconds (noon, so the day survives time zones).
export function dayInput(t: number | null | undefined) {
  if (!t) return ""
  const d = new Date(t * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}
export const fromDayInput = (v: string) => (v ? Math.floor(new Date(`${v}T12:00:00`).getTime() / 1000) : null)

// Whole minutes from an input; NaN when it isn't one.
export const toMinutes = (v: string) => (v.trim() === "" ? 0 : Number.isInteger(Number(v)) ? Number(v) : NaN)
