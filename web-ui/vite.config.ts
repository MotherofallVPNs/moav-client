import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    host: "0.0.0.0",
    port: 3000,
    // Same-origin /api in dev: forward to a locally-running proxy-core so the
    // dashboard code can use relative URLs (matching the nginx/Caddy prod path).
    proxy: {
      "/api": {
        target: "http://localhost:8088",
        changeOrigin: true,
        ws: true,
      },
    },
  },
  preview: {
    host: "0.0.0.0",
    port: 3000,
  },
});
