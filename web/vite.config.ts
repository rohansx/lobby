import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// The build lands inside the Go package so it is embedded into the binary.
// In dev, API and public routes are proxied to a local `lobby serve` on :8080.
const backend = process.env.LOBBY_DEV_BACKEND ?? "http://localhost:8080"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  build: { outDir: "../internal/app/web/dist", emptyOutDir: true },
  server: {
    proxy: Object.fromEntries(["/api", "/v1", "/w/", "/s/", "/u/", "/static", "/healthz"].map((p) => [p, backend])),
  },
})
