import path from "path";

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "/",
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 3000,
    host: "0.0.0.0",
  },
  build: {
    outDir: "dist",
    sourcemap: false,
    minify: "oxc",
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return;
          if (id.includes("/react/") || id.includes("/react-dom/")) {
            return "react";
          }
          if (id.includes("react-router-dom")) {
            return "router";
          }
          if (id.includes("recharts")) {
            return "charts";
          }
          if (id.includes("@dnd-kit")) {
            return "dnd";
          }
          if (
            id.includes("react-markdown") ||
            id.includes("rehype-sanitize") ||
            id.includes("remark-gfm")
          ) {
            return "markdown";
          }
          if (id.includes("framer-motion")) {
            return "motion";
          }
          if (
            id.includes("lucide-react") ||
            id.includes("react-hot-toast") ||
            id.includes("sonner")
          ) {
            return "ui";
          }
        },
      },
      treeshake: true,
    },
  },
});
