import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The API runs on customsvn serve; in development Vite proxies to it
// (CUSTOMSVN_API, default http://127.0.0.1:8080).
const apiTarget = process.env.CUSTOMSVN_API ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [tanstackRouter({ target: "react", autoCodeSplitting: true }), react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: {
    proxy: {
      // Host is kept so the server's same-origin check sees what the browser asked for.
      "/api": { target: apiTarget, changeOrigin: false },
      "/healthz": { target: apiTarget, changeOrigin: false },
      "/v1": { target: apiTarget, changeOrigin: false },
      "/mcp": { target: apiTarget, changeOrigin: false },
    },
  },
  build: { outDir: "dist", emptyOutDir: true, chunkSizeWarningLimit: 800 },
  test: { environment: "node", include: ["src/**/*.test.ts"] },
});
