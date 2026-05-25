import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Vite dev server (port 5173) proxies /api/* and /healthz to the Go hub on
// :8090 so the same frontend code works in dev (via proxy) and in prod
// (served from the hub's embed.FS). WebSocket upgrades are proxied too.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8090",
        changeOrigin: true,
        ws: true,
      },
      "/healthz": {
        target: "http://localhost:8090",
        changeOrigin: true,
      },
    },
  },
});
