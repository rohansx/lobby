import { FileIcon, LoaderIcon, Trash2Icon, UploadIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { api, projectPath, type Asset, type AssetKind } from "@/lib/api"
import { toastError } from "@/lib/errors"
import { num } from "@/lib/format"
import { useApi } from "@/lib/use-api"

const kinds: AssetKind[] = ["logo", "screenshot", "banner", "demo", "other"]
const accept = "image/png,image/jpeg,image/webp,image/gif,application/pdf"

const size = (b: number) => (b >= 1 << 20 ? `${(b / (1 << 20)).toFixed(1)} MB` : `${num(Math.ceil(b / 1024))} KB`)

export function AssetGrid({ slug, editable }: { slug: string; editable: boolean }) {
  const path = projectPath(slug, "/assets")
  const { data, reload } = useApi<{ data: Asset[]; max: number; max_bytes: number }>(path)
  const [kind, setKind] = useState<AssetKind>("screenshot")
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)

  async function upload(files: FileList | null) {
    if (!files?.length) return
    setBusy(true)
    try {
      // One request per file so a bad file doesn't sink the rest.
      for (const f of Array.from(files)) {
        const body = new FormData()
        body.set("kind", kind)
        body.set("file", f)
        await api(path, { body }).catch((err) => toastError(new Error(`${f.name}: ${(err as Error).message}`)))
      }
      await reload()
    } finally {
      setBusy(false)
      if (input.current) input.current.value = ""
    }
  }

  async function remove(a: Asset) {
    try {
      await api(projectPath(slug, `/assets/${a.id}`), { method: "DELETE" })
      await reload()
    } catch (err) {
      toastError(err)
    }
  }

  const full = !!data && data.data.length >= data.max
  return (
    <Card>
      <CardHeader>
        <CardTitle>Assets</CardTitle>
        <CardDescription>
          Logo, screenshots, banners and a demo. PNG, JPEG, WebP, GIF or PDF, up to {data ? size(data.max_bytes) : "5 MB"} each
          {data && `, ${data.data.length} of ${data.max} used`}.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {editable && (
          <div className="flex flex-wrap items-end gap-2">
            <div className="grid gap-2">
              <Label htmlFor="asset-kind">Kind</Label>
              <Select value={kind} onValueChange={(v) => setKind(v as AssetKind)}>
                <SelectTrigger id="asset-kind" className="w-40 capitalize">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {kinds.map((k) => (
                    <SelectItem key={k} value={k} className="capitalize">
                      {k}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <input ref={input} type="file" accept={accept} multiple hidden onChange={(e) => upload(e.target.files)} />
            <Button type="button" variant="outline" disabled={busy || full} onClick={() => input.current?.click()}>
              {busy ? <LoaderIcon className="animate-spin" /> : <UploadIcon />}
              Upload
            </Button>
          </div>
        )}
        {!data ? (
          <Skeleton className="h-32" />
        ) : data.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No assets yet.</p>
        ) : (
          <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {data.data.map((a) => (
              <AssetTile key={a.id} asset={a} src={"/api/admin" + projectPath(slug, `/assets/${a.id}`)} onDelete={editable ? () => remove(a) : undefined} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function AssetTile({ asset: a, src, onDelete }: { asset: Asset; src: string; onDelete?: () => void }) {
  return (
    <li className="overflow-hidden rounded-lg border">
      <a href={src} target="_blank" rel="noreferrer" className="flex aspect-video items-center justify-center bg-muted/40">
        {a.mime.startsWith("image/") ? <img src={src} alt={a.filename} loading="lazy" className="size-full object-contain" /> : <FileIcon className="size-8 text-muted-foreground" />}
      </a>
      <div className="flex items-center gap-2 px-2 py-1.5">
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-medium" title={a.filename}>
            {a.filename}
          </div>
          <div className="text-xs text-muted-foreground capitalize">
            {a.kind} · {size(a.size)}
          </div>
        </div>
        {onDelete && (
          <Button type="button" variant="ghost" size="icon-sm" aria-label={`Delete ${a.filename}`} onClick={onDelete}>
            <Trash2Icon />
          </Button>
        )}
      </div>
    </li>
  )
}
