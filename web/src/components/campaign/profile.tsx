import { LoaderIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { AssetGrid } from "@/components/campaign/assets"
import { ProfileForm } from "@/components/campaign/profile-form"
import { NewProjectDialog } from "@/components/new-project-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { api, type Campaign } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { useSession } from "@/lib/session"

type Props = { campaign: Campaign; onChange: () => Promise<void> }

export function ProfileTab({ campaign: c, onChange }: Props) {
  const intake = c.status === "intake"
  const editable = c.status !== "done" && c.status !== "cancelled"
  return (
    <div className="grid gap-6">
      {intake && <WaitlistPicker campaign={c} onChange={onChange} />}
      {c.project_slug ? (
        <>
          <ProfileForm key={c.project_slug} slug={c.project_slug} editable={editable} />
          <AssetGrid key={`a-${c.project_slug}`} slug={c.project_slug} editable={editable} />
          {intake && <SubmitIntake campaign={c} onChange={onChange} />}
        </>
      ) : (
        !intake && <p className="text-sm text-muted-foreground">This campaign has no waitlist any more.</p>
      )}
    </div>
  )
}

function WaitlistPicker({ campaign: c, onChange }: Props) {
  const { projects } = useSession()
  const [busy, setBusy] = useState(false)
  const choose = async (slug: string) => {
    setBusy(true)
    try {
      await api(`/campaigns/${c.id}/project`, { method: "PUT", body: { slug } })
      await onChange()
    } catch (err) {
      toastError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Waitlist</CardTitle>
        <CardDescription>The waitlist this product's launch sends people to. Signups from each destination are counted here.</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-2">
        {projects && projects.length > 0 && (
          <Select value={c.project_slug || undefined} onValueChange={choose} disabled={busy}>
            <SelectTrigger className="w-full sm:w-72" aria-label="Waitlist">
              <SelectValue placeholder="Choose a waitlist" />
            </SelectTrigger>
            <SelectContent>
              {projects.map((p) => (
                <SelectItem key={p.id} value={p.slug}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        <NewProjectDialog onCreated={(p) => choose(p.slug)}>
          <Button variant="outline">
            <PlusIcon />
            New waitlist
          </Button>
        </NewProjectDialog>
      </CardContent>
    </Card>
  )
}

function SubmitIntake({ campaign: c, onChange }: Props) {
  const [busy, setBusy] = useState(false)
  const submit = async () => {
    setBusy(true)
    try {
      await api(`/campaigns/${c.id}/submit-intake`, { body: {} })
      toast.success("Sent for the readiness review.")
      await onChange()
    } catch (err) {
      toastError(err)
      setBusy(false)
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Ready for review?</CardTitle>
        <CardDescription>Save the profile first. Missing fields are fine; the review lists what's needed. You can keep editing the profile and assets afterwards.</CardDescription>
      </CardHeader>
      <CardFooter className="border-t">
        <Button disabled={busy} onClick={submit}>
          {busy && <LoaderIcon className="animate-spin" />}
          Submit for readiness review
        </Button>
      </CardFooter>
    </Card>
  )
}
