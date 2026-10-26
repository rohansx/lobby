import { useState, type ReactNode } from "react"
import { useLocation } from "wouter"
import { SecretKeyDialog } from "@/components/secret-key-dialog"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { api, type Project } from "@/lib/api"
import { useSession } from "@/lib/session"

const slugify = (s: string) =>
  s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)

export function NewProjectDialog({ children }: { children: ReactNode }) {
  const { refreshProjects } = useSession()
  const [, navigate] = useLocation()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [slug, setSlug] = useState("")
  const [slugEdited, setSlugEdited] = useState(false)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(o) => {
          setOpen(o)
          if (!o) {
            setName("")
            setSlug("")
            setSlugEdited(false)
            setError("")
          }
        }}
      >
        <DialogTrigger asChild>{children}</DialogTrigger>
        <DialogContent>
          <form
            className="grid gap-4"
            onSubmit={async (e) => {
              e.preventDefault()
              setBusy(true)
              setError("")
              try {
                const r = await api<{ project: Project; secret_key: string }>("/projects", { body: { name, slug: slug || slugify(name) } })
                await refreshProjects()
                setOpen(false)
                setSecret(r.secret_key)
                navigate(`/p/${r.project.slug}`)
              } catch (err) {
                setError((err as Error).message)
              } finally {
                setBusy(false)
              }
            }}
          >
            <DialogHeader>
              <DialogTitle>New waitlist</DialogTitle>
              <DialogDescription>One project per product. You can change everything except the slug later.</DialogDescription>
            </DialogHeader>
            <div className="grid gap-2">
              <Label htmlFor="np-name">Name</Label>
              <Input
                id="np-name"
                required
                autoFocus
                placeholder="Convox"
                value={name}
                onChange={(e) => {
                  setName(e.target.value)
                  if (!slugEdited) setSlug(slugify(e.target.value))
                }}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="np-slug">Slug</Label>
              <Input
                id="np-slug"
                placeholder="convox"
                value={slug}
                onChange={(e) => {
                  setSlugEdited(true)
                  setSlug(e.target.value)
                }}
              />
              <p className="text-xs text-muted-foreground">Used in the hosted page URL: /w/{slug || "slug"}</p>
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <DialogFooter>
              <Button type="submit" disabled={busy || !name.trim()}>
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <SecretKeyDialog secret={secret} onClose={() => setSecret(null)} />
    </>
  )
}
