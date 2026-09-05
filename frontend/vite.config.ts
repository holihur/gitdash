import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    rollupOptions: {
      input: {
        main: path.resolve(__dirname, "index.html"),
        admin: path.resolve(__dirname, "admin.html"),
      },
      output: {
        // react 运行时单独分包：版本更新少、可长期缓存，且与业务代码并行加载
        manualChunks(id) {
          if (id.includes("node_modules") && /node_modules\/(react|react-dom|scheduler|react-router|@remix-run)\b/.test(id)) {
            return "vendor-react";
          }
        },
      },
    },
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
