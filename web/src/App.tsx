import { lazy, Suspense, useCallback, useEffect, useState } from "react"
import { Redirect, Route, Switch, useLocation } from "wouter"
import { AppShell } from "@/components/app-shell"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type Me, type ProjectSummary } from "@/lib/api"
import { SessionContext } from "@/lib/session"
import { AuthPage, type AuthMode } from "@/pages/auth"
import { Home } from "@/pages/home"

// Project pages are split out so the first paint (login or home) stays small.
const Overview = lazy(() => import("@/pages/overview"))
const Signups = lazy(() => import("@/pages/signups"))
const Webhooks = lazy(() => import("@/pages/webhooks"))
const Broadcast = lazy(() => import("@/pages/broadcast"))
const Settings = lazy(() => import("@/pages/settings"))
const BillingPage = lazy(() => import("@/pages/billing"))
const Checkout = lazy(() => import("@/pages/checkout"))

// Signed-out pages: cloud instances have open signup and password reset.
const publicModes: AuthMode[] = ["login", "signup", "forgot", "reset"]

export default function App() {
  const [me, setMe] = useState<Me | null>(null)
  const [projects, setProjects] = useState<ProjectSummary[]>()
  const [queue, setQueue] = useState({ queued: 0, failed: 0 })
  const [location] = useLocation()

  const refreshMe = useCallback(async () => {
    setMe(await api<Me>("/me"))
  }, [])

  const refreshProjects = useCallback(async () => {
    const r = await api<{ data: ProjectSummary[]; queued: number; failed: number }>("/projects")
    setProjects(r.data)
    setQueue({ queued: r.queued, failed: r.failed })
  }, [])

  useEffect(() => {
    refreshMe()
    const out = () => setMe((m) => (m ? { ...m, email: undefined } : m))
    window.addEventListener("lobby:signed-out", out)
    return () => window.removeEventListener("lobby:signed-out", out)
  }, [refreshMe])

  useEffect(() => {
    if (me?.email) refreshProjects()
  }, [me?.email, refreshProjects])

  if (!me) return null

  let body
  if (me.setup) {
    body = location === "/setup" ? <AuthPage mode="setup" onDone={refreshMe} me={me} /> : <Redirect to="/setup" replace />
  } else if (!me.email) {
    const mode = publicModes.find((m) => location === `/${m}` && (m === "login" || me.cloud))
    body = mode ? (
      <AuthPage key={mode} mode={mode} onDone={refreshMe} me={me} />
    ) : (
      // Someone heading to checkout is most likely new, so they get signup.
      <Redirect to={`/${location === "/checkout" && me.cloud ? "signup" : "login"}?next=${encodeURIComponent(location + window.location.search)}`} replace />
    )
  } else {
    body = (
      <AppShell>
        <Suspense fallback={null}>
          <Switch>
            <Route path="/">
              <Redirect to="/dashboard" replace />
            </Route>
            <Route path="/dashboard" component={Home} />
            <Route path="/login">
              <Redirect to={safeNext()} replace />
            </Route>
            {["/signup", "/forgot", "/reset"].map((p) => (
              <Route key={p} path={p}>
                <Redirect to={safeNext()} replace />
              </Route>
            ))}
            {me.cloud && <Route path="/checkout" component={Checkout} />}
            {me.cloud && <Route path="/billing" component={BillingPage} />}
            <Route path="/setup">
              <Redirect to="/dashboard" replace />
            </Route>
            <Route path="/p/:slug" component={Overview} />
            <Route path="/p/:slug/signups" component={Signups} />
            <Route path="/p/:slug/webhooks" component={Webhooks} />
            <Route path="/p/:slug/broadcast" component={Broadcast} />
            <Route path="/p/:slug/settings" component={Settings} />
            <Route>
              <div className="p-8 text-muted-foreground">Page not found.</div>
            </Route>
          </Switch>
        </Suspense>
      </AppShell>
    )
  }

  return (
    <SessionContext.Provider value={{ me, refreshMe, projects, queue, refreshProjects }}>
      <TooltipProvider delayDuration={200}>
        {body}
        <Toaster position="top-right" />
      </TooltipProvider>
    </SessionContext.Provider>
  )
}

// Only same-site paths are followed after sign-in.
function safeNext() {
  const next = new URLSearchParams(window.location.search).get("next") ?? "/dashboard"
  return next.startsWith("/") && !next.startsWith("//") && !next.startsWith("/\\") && next !== "/" ? next : "/dashboard"
}
