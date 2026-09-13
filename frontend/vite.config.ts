/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { visualizer } from "rollup-plugin-visualizer";
import path from "path";

export default defineConfig(({ mode }) => ({
  plugins: [
    react(),
    // `pnpm build:analyze`（vite build --mode analyze）生成 dist/stats.html 体积报告
    ...(mode === "analyze"
      ? [visualizer({ filename: "dist/stats.html", gzipSize: true, brotliSize: true, template: "treemap" })]
      : []),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
  },
  build: {
    rollupOptions: {
      input: {
        main: path.resolve(__dirname, "index.html"),
        admin: path.resolve(__dirname, "admin.html"),
      },
      output: {
        // React 运行时单独分包：版本更新少、可长期缓存，且与业务代码并行加载。
        // CodeMirror 及其语言包完全交给 Rollup 的动态 import 拆分（见
        // components/code-editor.tsx：按文件类型 import() 对应语言包），
        // 不在这里手动合并，以免破坏按需加载。
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
}));
