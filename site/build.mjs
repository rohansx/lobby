// Builds the docs and keeps the header and footer the same on every page.
//
//   node site/build.mjs
//
// Docs pages are written as HTML fragments in site/docs-src/. Each starts with
// a front-matter comment, <!-- {"title": "...", "description": "..."} -->, and
// is listed in `sections` below, which sets the sidebar order and the URL.
// Every other page in site/ gets its header and footer replaced between the
// <!-- header --> … <!-- /header --> and <!-- footer --> … <!-- /footer --> markers.
// The output is committed, so the nginx image needs no build step.
import { readFileSync, writeFileSync, mkdirSync, readdirSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const root = dirname(fileURLToPath(import.meta.url))

const sections = [
  { name: "Getting started", pages: [["index", "Introduction"], ["quickstart", "Quick start"]] },
  { name: "Guides", pages: [["embed", "Add it to your site"], ["waitlists", "How the line works"], ["invites", "Invites and launch day"], ["email", "Email"]] },
  { name: "Reference", pages: [["api", "REST API"], ["webhooks", "Webhooks"], ["cli", "CLI"]] },
  { name: "Running Lobby", pages: [["self-hosting", "Self-hosting"], ["cloud", "Lobby Cloud"]] },
]
const order = sections.flatMap((s) => s.pages.map(([slug, label]) => ({ slug, label, section: s.name })))
const href = (slug) => (slug === "index" ? "/docs" : `/docs/${slug}`)

const navLinks = [
  ["/docs", "Docs"],
  ["/pricing", "Pricing"],
  ["/docs/self-hosting", "Self-host"],
]

const gh = `<svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>`

// current is the path of the page, e.g. "/pricing" or "/docs/api".
function header(current) {
  const active = (path) =>
    path === "/docs" ? current === "/docs" || (current.startsWith("/docs/") && current !== "/docs/self-hosting") : current === path
  const links = navLinks.map(([p, l]) => `<a href="${p}"${active(p) ? ' aria-current="page"' : ""}>${l}</a>`).join("\n      ")
  return `<!-- header -->
<header class="nav" id="nav">
  <div class="wrap">
    <a class="logo" href="/"><img src="/logo.svg" alt="">Lobby</a>
    <nav class="nav-links" aria-label="Main">
      ${links}
    </nav>
    <div class="nav-right">
      <a class="gh" href="https://github.com/rohansx/lobby" aria-label="Lobby on GitHub">
        ${gh}
        <span>GitHub</span><b data-stars hidden></b>
      </a>
      <a class="signin" href="/login">Sign in</a>
      <a class="btn btn-primary" href="/signup">Get started
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg></a>
      <details class="menu">
        <summary aria-label="Menu"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16"/></svg></summary>
        <nav aria-label="Main">
          ${links}
          <a href="/login">Sign in</a>
        </nav>
      </details>
    </div>
  </div>
</header>
<!-- /header -->`
}

const footer = `<!-- footer -->
<footer>
  <div class="wrap">
    <a class="logo" href="/"><img src="/logo.svg" alt="">Lobby</a>
    <span>MIT licensed. Made by <a class="inline-link" href="https://github.com/rohansx">@rohansx</a>.</span>
    <nav>
      <a href="/docs">Docs</a>
      <a href="/pricing">Pricing</a>
      <a href="/changelog">Changelog</a>
      <a href="https://github.com/rohansx/lobby">GitHub</a>
      <a href="mailto:support@trylobby.app">Support</a>
      <a href="/terms">Terms</a>
      <a href="/privacy">Privacy</a>
    </nav>
  </div>
</footer>
<!-- /footer -->`

function head({ title, description, path, css = [] }) {
  return `<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${title}</title>
<meta name="description" content="${esc(description)}">
<meta name="theme-color" content="#fbfaf7">
<meta property="og:title" content="${esc(title)}">
<meta property="og:description" content="${esc(description)}">
<meta property="og:image" content="https://trylobby.app/og.png">
<meta name="twitter:card" content="summary_large_image">
<meta property="og:url" content="https://trylobby.app${path}">
<link rel="canonical" href="https://trylobby.app${path}">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Geist:wght@300..600&family=Geist+Mono:wght@400;500&display=swap" rel="stylesheet">
<link rel="stylesheet" href="/site.css">
${css.map((c) => `<link rel="stylesheet" href="${c}">`).join("\n")}`
}

const esc = (s) => s.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;")
const slugify = (s) =>
  s.replace(/<[^>]+>/g, "").toLowerCase().replace(/&[a-z]+;/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")

function frontMatter(src, file) {
  const m = src.match(/^<!--\s*(\{[\s\S]*?\})\s*-->\n?/)
  if (!m) throw new Error(`${file}: missing front matter`)
  return [JSON.parse(m[1]), src.slice(m[0].length)]
}

// Give every h2 and h3 an id and a hover anchor, and collect the h2s for "On this page".
function anchor(body) {
  const toc = []
  const seen = new Set()
  const out = body.replace(/<(h[23])>([\s\S]*?)<\/\1>/g, (_, tag, text) => {
    let id = slugify(text)
    while (seen.has(id)) id += "-"
    seen.add(id)
    if (tag === "h2") toc.push([id, text.replace(/<[^>]+>/g, "")])
    return `<${tag} id="${id}"><a class="anchor" href="#${id}" aria-hidden="true">#</a>${text}</${tag}>`
  })
  return [out, toc]
}

function sidebar(current) {
  return sections
    .map(
      (s) => `<div class="side-group">
        <p>${s.name}</p>
        ${s.pages.map(([slug, label]) => `<a href="${href(slug)}"${slug === current ? ' aria-current="page"' : ""}>${label}</a>`).join("\n        ")}
      </div>`,
    )
    .join("\n      ")
}

function docPage(slug) {
  const file = join(root, "docs-src", `${slug}.html`)
  const [meta, raw] = frontMatter(readFileSync(file, "utf8"), file)
  const [body, toc] = anchor(raw.trim())
  const i = order.findIndex((p) => p.slug === slug)
  const prev = order[i - 1], next = order[i + 1]
  const path = href(slug)
  const pager = `<nav class="pager" aria-label="Previous and next">
        ${prev ? `<a href="${href(prev.slug)}"><span>Previous</span>${prev.label}</a>` : "<i></i>"}
        ${next ? `<a class="next" href="${href(next.slug)}"><span>Next</span>${next.label}</a>` : ""}
      </nav>`
  const title = slug === "index" ? "Lobby docs" : `${meta.title} · Lobby docs`
  return `<!doctype html>
<html lang="en">
<head>
<!-- Generated by site/build.mjs from site/docs-src/${slug}.html. Edit that file, not this one. -->
${head({ title, description: meta.description, path, css: ["/docs.css"] })}
</head>
<body>
<div class="page">

${header(path)}

<main class="wrap docs">
  <aside class="docs-side" aria-label="Docs">
    <details class="docs-picker">
      <summary><span>${order[i].section}</span>${order[i].label}</summary>
      ${sidebar(slug)}
    </details>
    <div class="docs-tree">
      ${sidebar(slug)}
    </div>
  </aside>

  <article class="prose">
    <p class="crumb">${order[i].section}</p>
    <h1>${meta.title}</h1>
    ${meta.lede ? `<p class="lede">${meta.lede}</p>` : ""}
    ${body}
    <div class="doc-foot">
      ${pager}
      <p class="edit"><a href="https://github.com/rohansx/lobby/edit/main/site/docs-src/${slug}.html">Edit this page on GitHub</a> · <a href="mailto:support@trylobby.app">Ask a question</a></p>
    </div>
  </article>

  <nav class="toc" aria-label="On this page">
    ${toc.length ? `<p>On this page</p>\n    ${toc.map(([id, t]) => `<a href="#${id}">${t}</a>`).join("\n    ")}` : ""}
  </nav>
</main>

${footer}
</div>
<script src="/site.js" defer></script>
</body>
</html>
`
}

for (const { slug } of order) {
  const out = slug === "index" ? join(root, "docs.html") : join(root, "docs", `${slug}.html`)
  mkdirSync(dirname(out), { recursive: true })
  writeFileSync(out, docPage(slug))
}

// Stamp the shared header and footer onto the hand-written pages.
for (const f of readdirSync(root).filter((f) => f.endsWith(".html") && f !== "docs.html")) {
  const file = join(root, f)
  const path = f === "index.html" ? "/" : "/" + f.replace(/\.html$/, "")
  const src = readFileSync(file, "utf8")
  const out = src
    .replace(/<!-- header -->[\s\S]*?<!-- \/header -->/, header(path))
    .replace(/<!-- footer -->[\s\S]*?<!-- \/footer -->/, footer)
  if (!out.includes("<!-- header -->") || !out.includes("<!-- footer -->")) throw new Error(`${f}: add the header and footer markers`)
  writeFileSync(file, out)
}
console.log(`built ${order.length} docs pages`)
