import { CheckIcon, CopyIcon } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export function CopyButton({ value, className }: { value: string; className?: string }) {
  const [done, setDone] = useState(false)
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      className={className}
      aria-label="Copy"
      onClick={async () => {
        await navigator.clipboard.writeText(value)
        setDone(true)
        setTimeout(() => setDone(false), 1500)
      }}
    >
      {done ? <CheckIcon /> : <CopyIcon />}
    </Button>
  )
}

export function CopyField({ value, multiline, className }: { value: string; multiline?: boolean; className?: string }) {
  return (
    <div className={cn("relative rounded-lg border bg-muted/40", className)}>
      <pre className={cn("overflow-x-auto px-3 py-2 pr-11 font-mono text-xs leading-relaxed", !multiline && "whitespace-nowrap")}>{value}</pre>
      <CopyButton value={value} className="absolute top-1 right-1" />
    </div>
  )
}
