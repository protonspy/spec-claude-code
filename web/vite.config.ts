import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The build lands in the Go package that embeds it. `npm run dev` proxies the API
// to a running `scc view`, so the frontend can be worked on with hot reload.
export default defineConfig({
  plugins: [react()],
  base: "./",
  build: {
    outDir: "../internal/view/dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: { "/api": { target: "http://127.0.0.1:7777", changeOrigin: true } },
  },
});
