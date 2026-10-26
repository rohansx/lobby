import { Link } from "wouter"
import { Button } from "@/components/ui/button"

export function LoadError({ error }: { error: Error }) {
  return (
    <div className="mx-auto max-w-md px-4 py-16 text-center">
      <p className="font-medium">{error.message === "project not found" ? "There's no project here." : error.message}</p>
      <Button asChild variant="outline" className="mt-4">
        <Link href="/">All projects</Link>
      </Button>
    </div>
  )
}
