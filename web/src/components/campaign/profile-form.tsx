import { LoaderIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { api, projectPath, type ProductProfile, type ProfileVersion, type ProfileView } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { dateTime } from "@/lib/format"
import { useApi } from "@/lib/use-api"
import { cn } from "@/lib/utils"

type Key = keyof ProductProfile
type FieldDef = { key: Key; label: string; max: number; type?: "url" | "email" | "area" | "stage"; hint?: string }

// Mirrors the limits in internal/app/profiles.go.
const fields: FieldDef[] = [
  { key: "name", label: "Product name", max: 120 },
  { key: "url", label: "Product URL", max: 500, type: "url" },
  { key: "tagline", label: "Tagline", max: 60, hint: "One line, no hype." },
  { key: "short_description", label: "Short description", max: 160, type: "area" },
  { key: "long_description", label: "Long description", max: 1000, type: "area" },
  { key: "audience", label: "Who it's for", max: 500, type: "area" },
  { key: "problem", label: "The problem it solves", max: 1000, type: "area" },
  { key: "category", label: "Category", max: 100, hint: "For example: developer tools, analytics, note taking." },
  { key: "stage", label: "Stage", max: 20, type: "stage", hint: "Some destinations only accept products at certain stages." },
  { key: "pricing", label: "Pricing", max: 300, hint: "What it costs today, including free tiers or trials." },
  { key: "availability", label: "Availability", max: 300, hint: "Who can use it now: open to all, invite only, regions, platforms." },
  { key: "demo_url", label: "Demo URL", max: 500, type: "url", hint: "A video or a live demo people can try." },
  { key: "founder_name", label: "Founder name", max: 120 },
  { key: "founder_email", label: "Founder email", max: 254, type: "email" },
  { key: "founder_links", label: "Founder links", max: 1000, type: "area", hint: "One URL per line: X, LinkedIn, GitHub, a personal site." },
]

const empty = Object.fromEntries(fields.map((f) => [f.key, ""])) as unknown as ProductProfile
const length = (s: string) => [...s].length

export function ProfileForm({ slug, editable }: { slug: string; editable: boolean }) {
  const path = projectPath(slug, "/profile")
  const { data, reload } = useApi<ProfileView>(path)
  const [edits, setForm] = useState<ProductProfile | null>(null)
  const [saving, setSaving] = useState(false)
  const form = edits ?? (data ? { ...empty, ...data.latest?.profile } : null)

  if (!data || !form) return <Skeleton className="h-96" />
  const missing = fields.filter((f) => form[f.key].trim() === "")
  const tooLong = fields.some((f) => length(form[f.key]) > f.max)

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      const r = await api<{ version: ProfileVersion }>(path, { body: form })
      setForm({ ...empty, ...r.version.profile })
      await reload()
      toast.success(`Saved as version #${r.version.id}`)
    } catch (err) {
      toastError(err)
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={save}>
      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            Product profile
            <VersionBadge view={data} />
          </CardTitle>
          <CardDescription>The one description every submission is written from. Leave a field empty rather than guess; nothing is made up for you. Each save keeps a new version.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-5">
          {missing.length > 0 && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-sm">
              <span className="font-medium">Still missing:</span> {missing.map((f) => f.label).join(", ")}
            </div>
          )}
          <fieldset disabled={!editable} className="grid gap-5 sm:grid-cols-2">
            {fields.map((f) => (
              <ProfileField key={f.key} def={f} value={form[f.key]} onChange={(v) => setForm({ ...form, [f.key]: v })} />
            ))}
          </fieldset>
        </CardContent>
        {editable && (
          <CardFooter className="border-t">
            <Button type="submit" disabled={saving || tooLong}>
              {saving && <LoaderIcon className="animate-spin" />}
              Save new version
            </Button>
          </CardFooter>
        )}
      </Card>
    </form>
  )
}

function VersionBadge({ view }: { view: ProfileView }) {
  if (!view.latest) return <Badge variant="outline">Not saved yet</Badge>
  const approved = view.approved?.id === view.latest.id
  return (
    <span className="flex flex-wrap items-center gap-2 text-xs font-normal text-muted-foreground">
      <Badge variant="outline">Version #{view.latest.id}</Badge>
      saved {dateTime(view.latest.created_at)}
      {approved ? <Badge variant="secondary">Approved</Badge> : view.approved && <span>· approved version is #{view.approved.id}</span>}
    </span>
  )
}

function ProfileField({ def, value, onChange }: { def: FieldDef; value: string; onChange: (v: string) => void }) {
  const id = `profile-${def.key}`
  const n = length(value)
  const wide = def.type === "area"
  return (
    <div className={cn("grid content-start gap-2", wide && "sm:col-span-2")}>
      <div className="flex items-baseline justify-between gap-2">
        <Label htmlFor={id}>{def.label}</Label>
        <span className={cn("text-xs tabular-nums", n > def.max ? "text-destructive" : "text-muted-foreground")}>
          {n}/{def.max}
        </span>
      </div>
      {def.type === "stage" ? (
        <Select value={value || undefined} onValueChange={onChange}>
          <SelectTrigger id={id} className="w-full">
            <SelectValue placeholder="Choose a stage" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="idea">Idea</SelectItem>
            <SelectItem value="prelaunch">Pre-launch</SelectItem>
            <SelectItem value="beta">Beta</SelectItem>
            <SelectItem value="live">Live</SelectItem>
          </SelectContent>
        </Select>
      ) : def.type === "area" ? (
        <Textarea id={id} rows={def.max > 300 ? 4 : 2} value={value} onChange={(e) => onChange(e.target.value)} aria-invalid={n > def.max} />
      ) : (
        <Input id={id} type={def.type ?? "text"} value={value} onChange={(e) => onChange(e.target.value)} aria-invalid={n > def.max} placeholder={def.type === "url" ? "https://" : undefined} />
      )}
      {def.hint && <p className="text-xs text-muted-foreground">{def.hint}</p>}
    </div>
  )
}
