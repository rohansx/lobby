import type { ReactNode } from "react"
import { CopyField } from "@/components/copy-field"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"

const secretKeyHelp = (
  <>
    This is the only time it's shown. Use it as <code className="font-mono">Authorization: Bearer sk_…</code> for the API and as <code className="font-mono">LOBBY_KEY</code> for the CLI. Keep it on the server, never in a browser.
  </>
)

export function SecretKeyDialog({ secret, onClose, title = "Copy your secret key", description = secretKeyHelp }: { secret: string | null; onClose: () => void; title?: string; description?: ReactNode }) {
  return (
    <Dialog open={!!secret} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {secret && <CopyField value={secret} />}
        <DialogFooter>
          <Button onClick={onClose}>I've saved it</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
