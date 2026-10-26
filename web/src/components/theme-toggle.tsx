import { MoonIcon, SunIcon } from "lucide-react"
import { Button } from "@/components/ui/button"

export function ThemeToggle() {
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="Toggle theme"
      onClick={() => {
        const dark = document.documentElement.classList.toggle("dark")
        try {
          localStorage.setItem("lobby-theme", dark ? "dark" : "light")
        } catch {
          /* storage unavailable */
        }
      }}
    >
      <SunIcon className="hidden dark:block" />
      <MoonIcon className="dark:hidden" />
    </Button>
  )
}
