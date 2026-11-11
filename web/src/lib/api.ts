export type Status = "pending" | "confirmed" | "invited" | "joined" | "removed"
export const STATUSES: Status[] = ["pending", "confirmed", "invited", "joined", "removed"]

export interface Me {
  setup: boolean
  cloud: boolean
  providers: ("google" | "github")[]
  email?: string
  verified?: boolean
  suspended?: boolean
  doi_locked?: boolean // cloud: double opt-in is always on
  operator?: boolean // cloud: email is in LOBBY_OPERATORS
  plan?: Plan
  mailer: string
  from: string
  base_url: string
}

// Cloud plans; -1 means unlimited.
export interface Plan {
  id: "free" | "pack" | "launch" | "pro" | "self"
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
  packs: Campaign[]
  billing_enabled: boolean
  subscriber: boolean // a grandfathered launch/pro subscription
}

export interface OpsAccount {
  id: number
  email: string
  verified: boolean
  plan: string
  interval: "" | "month" | "year"
  status: string
  created_at: number
  projects: number
  signups: number
  emails_month: number
  suspended: boolean
}

export interface OpsData {
  totals: {
    accounts: number
    verified: number
    paying: Record<string, number>
    mrr: number
    signups: number
    signups_7d: number
    emails_month: number
    queued_jobs: number
    failed_jobs: number
    accounts_shown: number
    truncated: boolean
  }
  accounts: OpsAccount[]
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
  qualification_question: string // cloud: one optional question on the hosted page
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
  answer?: string // reply to the qualification question
  relevant?: boolean // founder's review; absent means not reviewed
  feedback: boolean // sees the feedback widget
  feedback_set: boolean // switched by hand rather than following the project's audience
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

// Launch Pack (cloud only).
export type CampaignStatus = "intake" | "readiness" | "planning" | "awaiting_approval" | "executing" | "reporting" | "done" | "cancelled"

export const campaignLabel: Record<CampaignStatus, string> = {
  intake: "Intake",
  readiness: "Readiness review",
  planning: "Planning",
  awaiting_approval: "Your approval",
  executing: "Executing",
  reporting: "Reporting",
  done: "Done",
  cancelled: "Cancelled",
}

export interface ReadinessItem {
  label: string
  ok: boolean
  note: string
}

export interface Campaign {
  id: number
  project_id: number | null
  project_slug: string
  owner_id: number
  owner_email: string
  status: CampaignStatus
  price_cents: number
  paid_at: number | null
  readiness: ReadinessItem[]
  profile_version_id: number | null
  approved_at: number | null
  changes_note: string
  report_note: string // empty until the report is published
  report_published_at: number | null
  tasks: number
  created_at: number
  updated_at: number
}

export type TaskState = "planned" | "needs_assets" | "awaiting_approval" | "awaiting_founder" | "ready" | "submitted" | "live" | "rejected" | "blocked"

export interface CampaignTask {
  id: number
  campaign_id: number
  destination_id: number
  destination_slug: string
  destination_name: string
  destination_url: string
  owner: "staff" | "founder"
  state: TaskState
  reason: string
  fee_cents: number
  instructions: string
  submitted_at: number | null
  live_at: number | null
  listing_url: string
  evidence: string
  exception_note: string
  campaign_link: string // hosted page tagged for this destination; empty without a waitlist
  created_at: number
  updated_at: number
}

export interface CampaignDetail {
  campaign: Campaign
  tasks: CampaignTask[]
}

export interface ReportRow {
  kind: "task" | "unknown" | "other" | "total"
  task_id?: number
  label: string
  source: string
  destination_url?: string
  owner?: "staff" | "founder"
  state?: TaskState
  submitted_at?: number
  live_at?: number
  listing_url?: string
  fee_cents: number
  visits: number | null // null: no visits recorded, not zero
  signups: number
  confirmed: number
  relevant: number
  conversion: number | null // signups / visits, only when visits > 0
}

export interface CampaignReport {
  campaign: Campaign
  from: number | null // null until the plan is approved
  to: number
  visits_tracked: boolean
  rows: ReportRow[]
  unknown: ReportRow
  other: ReportRow
  total: ReportRow
  pack_price_cents: number
  platform_fees_cents: number
  attributed_confirmed: number
  cost_per_confirmed_cents: number | null
}

export interface ProductProfile {
  name: string
  url: string
  audience: string
  problem: string
  category: string
  pricing: string
  stage: "" | "idea" | "prelaunch" | "beta" | "live"
  availability: string
  founder_name: string
  founder_email: string
  founder_links: string // one URL per line
  tagline: string
  short_description: string
  long_description: string
  demo_url: string
}

export interface ProfileVersion {
  id: number
  profile: ProductProfile
  created_at: number
  approved_at: number | null
}

export interface ProfileView {
  latest: ProfileVersion | null
  approved: ProfileVersion | null
  missing: (keyof ProductProfile)[]
}

export type AssetKind = "logo" | "screenshot" | "banner" | "demo" | "other"

export interface Asset {
  id: number
  kind: AssetKind
  filename: string
  mime: string
  size: number
  created_at: number
}

// Operator views (LOBBY_OPERATORS) of the same records, with the fields founders don't see.
export type ProfileStage = "idea" | "prelaunch" | "beta" | "live"
export const PROFILE_STAGES: ProfileStage[] = ["idea", "prelaunch", "beta", "live"]

export interface Destination {
  id: number
  slug: string
  name: string
  url: string
  audience: string
  stages: string // comma-separated ProfileStage values; empty accepts any
  fee_cents: number
  fee_note: string
  fields: string
  rules: string
  mode: "staff" | "founder" | "guided"
  active: boolean
  last_verified_at: number | null
  notes: string
  created_at: number
  updated_at: number
}

export interface OpsCampaign extends Campaign {
  operator_minutes?: number
  task_states?: Partial<Record<TaskState, number>>
  task_minutes?: number
}

export interface OpsTask extends CampaignTask {
  minutes?: number
}

export interface OpsCampaignDetail {
  campaign: OpsCampaign
  tasks: OpsTask[]
  profile: ProfileView | null
  assets: Asset[]
  moves: CampaignStatus[] // statuses the server lets an operator move to from here
}

// Feedback: reports from invited users grouped into issues per element and route.
export type IssueType = "bug" | "confusing" | "idea"
export type IssueState = "open" | "approved" | "fixed" | "dismissed"
export const ISSUE_TYPES: IssueType[] = ["bug", "confusing", "idea"]
export const ISSUE_STATES: IssueState[] = ["open", "approved", "fixed", "dismissed"]

export interface Issue {
  id: number
  title: string
  type: IssueType
  state: IssueState
  path: string
  selector: string
  element_text: string
  agent_note: string
  agent_note_at: number | null
  handoffs: number
  handoff_at: number | null
  approved_at: number | null
  fixed_at: number | null
  closed_at: number | null
  created_at: number
  updated_at: number
  reporters: number
  reports: number
}

export interface FeedbackReport {
  id: number
  issue_id: number
  signup_id: number
  type: IssueType
  comment: string
  origin: string // e.g. https://app.example.com, empty when unknown
  path: string
  selector: string
  element_tag: string
  element_text: string
  viewport: string
  browser: string
  console_errors: string[] | null
  failed_requests: { path: string; status: number }[] | null
  release: string
  created_at: number
  email?: string
  source?: string
  wave?: string
}

export interface FeedbackSettings {
  console: boolean
  agent_key_set: boolean
  audience: "invited" | "selected"
}

export interface FeedbackList {
  data: Issue[] | null // ranked by reporters
  waves: string[] | null // invite dates, YYYY-MM-DD
  sources: string[] | null
  settings: FeedbackSettings
  api: boolean // API, CLI and agent key are on this plan
  reports: number
  report_limit: number // -1 means unlimited
}

export interface IssueDetail {
  issue: Issue
  reports: FeedbackReport[] | null
  packet: unknown
  packet_md: string
  replies: { sent: number; works: number; broken: number }
  blocked: number[] | null // signup ids that can't report
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

// A FormData body is sent as multipart, anything else as JSON.
export async function api<T = unknown>(path: string, init?: { method?: string; body?: unknown }): Promise<T> {
  const form = init?.body instanceof FormData
  const res = await fetch("/api/admin" + path, {
    method: init?.method ?? (init?.body === undefined ? "GET" : "POST"),
    headers: init?.body === undefined || form ? undefined : { "Content-Type": "application/json" },
    body: init?.body === undefined ? undefined : form ? (init.body as FormData) : JSON.stringify(init.body),
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
