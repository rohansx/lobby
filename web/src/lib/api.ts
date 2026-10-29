export type Status = "pending" | "confirmed" | "invited" | "joined" | "removed"
export const STATUSES: Status[] = ["pending", "confirmed", "invited", "joined", "removed"]

export interface Me {
  setup: boolean
  cloud: boolean
  providers: ("google" | "github")[]
  email?: string
  verified?: boolean
  plan?: Plan
  mailer: string
  from: string
  base_url: string
}

// Cloud plans; -1 means unlimited.
export interface Plan {
  id: "free" | "launch" | "pro" | "self"
  name: string
  signups: number
  projects: number
  emails: number
  webhooks: boolean
  branding: boolean
  monthly: number
  yearly: number
}

export interface Billing {
  account: {
    email: string
    verified: boolean
    has_password: boolean
    plan: string
    interval: "" | "month" | "year"
    status: string
    period_end: number | null
    cancel_at_period_end: boolean
  }
  plan: Plan
  usage: { signups: number; projects: number; emails: number }
  plans: Plan[]
  billing_enabled: boolean
}

export interface Project {
  id: number
  slug: string
  name: string
  public_key: string
  allowed_origins: string
  doi_enabled: boolean
  position_mode: "points" | "fifo"
  points_per_referral: number
  referral_ip_cap: number
  referral_hourly_cap: number
  show_position: boolean
  counter_floor: number
  from_name: string
  reply_to: string
  notify_email: string
  headline: string
  description: string
  site_url: string
  redirect_url: string
  invite_url: string
  consent_text: string
  consent_version: number
  created_at: number
}

export interface ProjectSummary extends Project {
  total: number
  confirmed: number
  today: number
}

export interface Stats {
  total: number
  by_status: Partial<Record<Status, number>>
  confirm_rate: number
  daily: { day: string; count: number }[] | null
  top_sources: { name: string; count: number }[] | null
  top_referrers: { id: number; email: string; count: number }[] | null
  referred: number
}

export interface Signup {
  id: number
  email: string
  status: Status
  base_position: number
  points: number
  referral_code: string
  referred_by: number | null
  source_url: string
  referrer: string
  utm_source: string
  utm_medium: string
  utm_campaign: string
  created_at: number
  confirmed_at: number | null
  invited_at: number | null
  joined_at: number | null
  unsubscribed_at: number | null
  referrals: number
  position: number
  status_url: string
}

export interface Webhook {
  id: number
  url: string
  secret?: string
  kind: "standard" | "slack"
  events: string
  active: boolean
  disabled_reason: string
  created_at: number
}

export interface Delivery {
  id: number
  webhook_id: number
  msg_id: string
  event: string
  payload: string
  status: string
  attempts: number
  next_at: number
  response_code: number
  last_error: string
  created_at: number
}

export interface Broadcast {
  id: number
  subject: string
  audience: string
  recipients: number
  created_at: number
}

export class ApiError extends Error {
  status: number
  upgrade: boolean
  constructor(status: number, message: string, upgrade = false) {
    super(message)
    this.status = status
    this.upgrade = upgrade
  }
}

export async function api<T = unknown>(path: string, init?: { method?: string; body?: unknown }): Promise<T> {
  const res = await fetch("/api/admin" + path, {
    method: init?.method ?? (init?.body === undefined ? "GET" : "POST"),
    headers: init?.body === undefined ? undefined : { "Content-Type": "application/json" },
    body: init?.body === undefined ? undefined : JSON.stringify(init.body),
    credentials: "same-origin",
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401 && !["/login", "/setup", "/signup", "/reset"].some((p) => path.startsWith(p))) {
      window.dispatchEvent(new Event("lobby:signed-out"))
    }
    throw new ApiError(res.status, data.error ?? res.statusText, res.status === 402)
  }
  return data as T
}

export const projectPath = (slug: string, rest = "") => `/projects/${encodeURIComponent(slug)}${rest}`
