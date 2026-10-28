import { ChevronsUpDownIcon, LayoutDashboardIcon, LogOutIcon, MegaphoneIcon, PlusIcon, SettingsIcon, UsersIcon, WebhookIcon } from "lucide-react"
import type { ReactNode } from "react"
import { Link, useLocation, useRoute } from "wouter"
import { NewProjectDialog } from "@/components/new-project-dialog"
import { ThemeToggle } from "@/components/theme-toggle"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { api } from "@/lib/api"
import { num } from "@/lib/format"
import { useSession } from "@/lib/session"

const sections = [
  { path: "", label: "Overview", icon: LayoutDashboardIcon },
  { path: "/signups", label: "Signups", icon: UsersIcon },
  { path: "/webhooks", label: "Webhooks", icon: WebhookIcon },
  { path: "/broadcast", label: "Broadcast", icon: MegaphoneIcon },
  { path: "/settings", label: "Settings", icon: SettingsIcon },
]

export function AppShell({ children }: { children: ReactNode }) {
  const { me, projects, queue, refreshMe } = useSession()
  const [location, navigate] = useLocation()
  const [, params] = useRoute<{ slug: string }>("/p/:slug/*?")
  const slug = params?.slug
  const current = projects?.find((p) => p.slug === slug)

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" asChild tooltip="All projects">
                <Link href="/">
                  <img src="/favicon.svg" alt="" className="size-8 rounded-lg dark:ring-1 dark:ring-white/15" />
                  <div className="grid flex-1 text-left leading-tight">
                    <span className="font-medium tracking-[-0.03em]">Lobby</span>
                    <span className="truncate text-xs text-muted-foreground">Waitlists</span>
                  </div>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          {current && (
            <SidebarGroup>
              <SidebarGroupLabel className="truncate">{current.name}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {sections.map((s) => {
                    const href = `/p/${current.slug}${s.path}`
                    return (
                      <SidebarMenuItem key={s.label}>
                        <SidebarMenuButton asChild isActive={location === href} tooltip={s.label}>
                          <Link href={href}>
                            <s.icon />
                            <span>{s.label}</span>
                          </Link>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    )
                  })}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          )}
          <SidebarGroup>
            <SidebarGroupLabel>Projects</SidebarGroupLabel>
            <NewProjectDialog>
              <SidebarGroupAction title="New project">
                <PlusIcon />
                <span className="sr-only">New project</span>
              </SidebarGroupAction>
            </NewProjectDialog>
            <SidebarGroupContent>
              <SidebarMenu>
                {!projects && [0, 1].map((i) => <SidebarMenuItem key={i}><SidebarMenuSkeleton /></SidebarMenuItem>)}
                {projects?.map((p) => (
                  <SidebarMenuItem key={p.id}>
                    <SidebarMenuButton asChild isActive={p.slug === slug} tooltip={p.name}>
                      <Link href={`/p/${p.slug}`}>
                        <span className="flex size-4 items-center justify-center rounded bg-muted text-[10px] font-semibold uppercase">{p.name[0]}</span>
                        <span className="truncate">{p.name}</span>
                      </Link>
                    </SidebarMenuButton>
                    <SidebarMenuBadge className="tabular-nums">{num(p.total)}</SidebarMenuBadge>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          {queue.failed > 0 && (
            <p className="px-2 text-xs text-destructive group-data-[collapsible=icon]:hidden">{queue.failed} emails failed to send. Check the server log.</p>
          )}
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuButton size="lg">
                    <div className="flex size-8 items-center justify-center rounded-lg bg-muted text-xs font-semibold uppercase">{me.email?.[0]}</div>
                    <div className="grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-medium">{me.email}</span>
                      <span className="truncate text-xs text-muted-foreground">{me.mailer.startsWith("log") ? "Email to log only" : `Email via ${me.mailer}`}</span>
                    </div>
                    <ChevronsUpDownIcon className="ml-auto size-4" />
                  </SidebarMenuButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent side="top" align="start" className="w-(--radix-dropdown-menu-trigger-width) min-w-56">
                  <DropdownMenuLabel className="font-normal">
                    <div className="text-xs text-muted-foreground">Sending from</div>
                    <div className="truncate">{me.from || "LOBBY_FROM_EMAIL not set"}</div>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onSelect={async () => {
                      await api("/logout", { method: "POST" })
                      await refreshMe()
                      navigate("/login")
                    }}
                  >
                    <LogOutIcon />
                    Sign out
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset className="min-w-0">
        <header className="sticky top-0 z-10 flex h-12 shrink-0 items-center gap-2 border-b bg-background/80 px-3 backdrop-blur">
          <SidebarTrigger />
          <div className="min-w-0 flex-1 truncate text-sm font-medium">{current?.name ?? (slug ? "" : "Projects")}</div>
          <ThemeToggle />
        </header>
        <div className="min-w-0 flex-1">{children}</div>
      </SidebarInset>
    </SidebarProvider>
  )
}
