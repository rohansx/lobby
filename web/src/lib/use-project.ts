import { useParams } from "wouter"
import { projectPath, type Project, type Stats } from "./api"
import { useApi } from "./use-api"

export interface ProjectDetail {
  project: Project
  stats: Stats
  snippets: { script: string; form: string; hosted: string }
}

export function useProject() {
  const { slug } = useParams<{ slug: string }>()
  return { slug, ...useApi<ProjectDetail>(projectPath(slug)) }
}
