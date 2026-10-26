import { CopyField } from "@/components/copy-field"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"

export function SecretKeyDialog({ secret, onClose }: { secret: string | null; onClose: () => void }) {
  return (
    <Dialog open={!!secret} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Copy your secret key</DialogTitle>
          <DialogDescription>
            This is the only time it's shown. Use it as <code className="font-mono">Authorization: Bearer sk_…</code> for the API and as <code className="font-mono">LOBBY_KEY</code> for the CLI. Keep it on the server, never in a browser.
          </DialogDescription>
        </DialogHeader>
        {secret && <CopyField value={secret} />}
        <DialogFooter>
          <Button onClick={onClose}>I've saved it</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
