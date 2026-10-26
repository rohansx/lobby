import { createContext, useContext } from "react"
import type { Me, ProjectSummary } from "./api"

export interface Session {
  me: Me
  refreshMe: () => Promise<void>
  projects: ProjectSummary[] | undefined
  queue: { queued: number; failed: number }
  refreshProjects: () => Promise<void>
}

export const SessionContext = createContext<Session | null>(null)

export function useSession() {
  const s = useContext(SessionContext)
  if (!s) throw new Error("useSession outside SessionContext")
  return s
}
