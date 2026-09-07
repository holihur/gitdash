import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./src",
  // 黑盒用例互相独立，避免并发时争抢同一个外部实例（GITDASH_UI_URL 模式）
  workers: 1,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  // 不需要 playwright test 管理的 webServer：实例由 fixtures/server.ts 负责
  use: {
    // 与前端 i18n 检测对齐（en => 英文文案，选择器依赖英文按钮文本）
    locale: "en-US",
    baseURL: process.env.GITDASH_UI_URL || "http://127.0.0.1:0",
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  reporter: [["list"]],
  outputDir: "./test-results",
});
