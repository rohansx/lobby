import { useEffect, useState, type ReactNode } from "react"
import { toast } from "sonner"
import { useLocation } from "wouter"
import { LoadError } from "@/components/not-found"
import { Page } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { api, projectPath, type Project } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { useSession } from "@/lib/session"
import { invalidate } from "@/lib/use-api"
import { useProject } from "@/lib/use-project"

export default function Settings() {
  const { slug, data, error, reload } = useProject()
  const { refreshProjects } = useSession()
  const [, navigate] = useLocation()
  const [form, setForm] = useState<Project | null>(null)
  const [saving, setSaving] = useState(false)
  const [confirmSlug, setConfirmSlug] = useState("")

  useEffect(() => {
    if (data && !form) setForm(data.project)
  }, [data, form])

  if (error && !data) return <LoadError error={error} />
  if (!form) return <Page title="Settings"><Skeleton className="h-96" /></Page>

  const set = <K extends keyof Project>(k: K, v: Project[K]) => setForm((f) => (f ? { ...f, [k]: v } : f))
  const text = (k: keyof Project) => ({ value: String(form[k] ?? ""), onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => set(k, e.target.value as never) })
  const int = (k: keyof Project) => ({ type: "number", min: 0, value: String(form[k] ?? 0), onChange: (e: React.ChangeEvent<HTMLInputElement>) => set(k, Number(e.target.value) as never) })

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      const p = await api<Project>(projectPath(slug), { method: "PATCH", body: form })
      setForm(p)
      invalidate(projectPath(slug))
      reload()
      refreshProjects()
      toast.success("Saved")
    } catch (err) {
      toastError(err)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Page title="Settings">
      <form onSubmit={save} className="grid gap-6">
        <Section title="Page" description="What visitors see on the hosted page and in emails.">
          <Field label="Name" id="name">
            <Input id="name" required {...text("name")} />
          </Field>
          <Field label="Headline" id="headline">
            <Input id="headline" placeholder={`Join the ${form.name} waitlist`} {...text("headline")} />
          </Field>
          <Field label="Description" id="description">
            <Textarea id="description" rows={3} {...text("description")} />
          </Field>
          <Field label="Consent text" id="consent" hint="Shown under the form. Changing it bumps the consent version stored with new signups.">
            <Textarea id="consent" rows={2} {...text("consent_text")} />
          </Field>
        </Section>

        <Section title="Links" description="Where people come from and where they go.">
          <Field label="Site URL" id="site_url" hint="Referral links point here with ?ref=. Leave empty to use the hosted page.">
            <Input id="site_url" type="url" placeholder="https://convox.ai" {...text("site_url")} />
          </Field>
          <Field label="Allowed origins" id="origins" hint="One per line. Signups from browsers on other sites are refused.">
            <Textarea id="origins" rows={3} className="font-mono text-xs" placeholder={"https://convox.ai\nhttps://www.convox.ai"} {...text("allowed_origins")} />
          </Field>
          <Field label="Redirect after signup" id="redirect_url" hint="For the plain HTML form. Leave empty to show the status page.">
            <Input id="redirect_url" type="url" {...text("redirect_url")} />
          </Field>
          <Field label="Invite URL" id="invite_url" hint="Where invite links land, with ?wl=<token>. Your app redeems it through the API.">
            <Input id="invite_url" type="url" placeholder="https://app.convox.ai/signup" {...text("invite_url")} />
          </Field>
        </Section>

        <Section title="Line" description="How the queue behaves.">
          <Toggle label="Double opt-in" hint="People confirm by email before they get a place. Strongly recommended." checked={form.doi_enabled} onChange={(v) => set("doi_enabled", v)} />
          <Toggle label="Show place in line" hint="On the status page and in emails." checked={form.show_position} onChange={(v) => set("show_position", v)} />
          <Field label="Ordering" id="position_mode">
            <Select value={form.position_mode} onValueChange={(v) => set("position_mode", v as Project["position_mode"])}>
              <SelectTrigger id="position_mode" className="w-full sm:w-72">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="points">Referrals move people up</SelectItem>
                <SelectItem value="fifo">First come, first served</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Places per referral" id="ppr">
              <Input id="ppr" {...int("points_per_referral")} />
            </Field>
            <Field label="Public counter starts at" id="floor" hint="The counter stays hidden below this.">
              <Input id="floor" {...int("counter_floor")} />
            </Field>
            <Field label="Referrals credited per IP" id="ipcap">
              <Input id="ipcap" {...int("referral_ip_cap")} />
            </Field>
            <Field label="Referrals credited per hour" id="hourcap">
              <Input id="hourcap" {...int("referral_hourly_cap")} />
            </Field>
          </div>
        </Section>

        <Section title="Email" description="The sender address itself comes from LOBBY_FROM_EMAIL.">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="From name" id="from_name">
              <Input id="from_name" placeholder={form.name} {...text("from_name")} />
            </Field>
            <Field label="Reply-to" id="reply_to">
              <Input id="reply_to" type="email" {...text("reply_to")} />
            </Field>
            <Field label="Notify me of new signups" id="notify_email" hint="Gets an email with the details of every new signup. Leave empty to turn off.">
              <Input id="notify_email" type="email" placeholder="you@example.com" {...text("notify_email")} />
            </Field>
          </div>
        </Section>

        <div className="sticky bottom-0 -mx-4 flex justify-end border-t bg-background/80 px-4 py-3 backdrop-blur md:-mx-8 md:px-8">
          <Button type="submit" disabled={saving}>
            Save changes
          </Button>
        </div>
      </form>

      <Card className="mt-8 border-destructive/40">
        <CardHeader>
          <CardTitle>Delete project</CardTitle>
          <CardDescription>Deletes the project, every signup, invite and webhook. This can't be undone. Type <span className="font-mono text-foreground">{slug}</span> to confirm.</CardDescription>
        </CardHeader>
        <CardFooter className="flex flex-wrap gap-2">
          <Input value={confirmSlug} onChange={(e) => setConfirmSlug(e.target.value)} placeholder={slug} className="w-56" aria-label="Project slug" />
          <Button
            variant="destructive"
            disabled={confirmSlug !== slug}
            onClick={async () => {
              try {
                await api(projectPath(slug, `?confirm=${encodeURIComponent(confirmSlug)}`), { method: "DELETE" })
                await refreshProjects()
                toast.success("Project deleted")
                navigate("/")
              } catch (err) {
                toastError(err)
              }
            }}
          >
            Delete project
          </Button>
        </CardFooter>
      </Card>
    </Page>
  )
}

function Section({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">{children}</CardContent>
    </Card>
  )
}

function Field({ label, id, hint, children }: { label: string; id: string; hint?: string; children: ReactNode }) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function Toggle({ label, hint, checked, onChange }: { label: string; hint: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-start justify-between gap-4">
      <span>
        <span className="text-sm font-medium">{label}</span>
        <span className="block text-xs text-muted-foreground">{hint}</span>
      </span>
      <Switch checked={checked} onCheckedChange={onChange} />
    </label>
  )
}
