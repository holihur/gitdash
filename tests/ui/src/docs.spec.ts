import { test, expect, type Page } from "@playwright/test";
import http from "node:http";
import { readFile, stat } from "node:fs/promises";
import path from "node:path";

/**
 * 文档站（Hugo，docs/public）的主题与语言切换黑盒测试。
 * 独立起一个静态服务，不需要 gitdash 实例；未构建文档时自动跳过。
 */

const PUBLIC_DIR = path.resolve(__dirname, "../../../docs/public");
const BASE_PATH = "/gitdash";
const CONTENT_TYPES: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json",
  ".xml": "application/xml",
  ".txt": "text/plain; charset=utf-8",
  ".svg": "image/svg+xml",
};

let server: http.Server | undefined;
let base = "";
let available = false;

test.beforeAll(async () => {
  try {
    await stat(path.join(PUBLIC_DIR, "index.html"));
  } catch {
    return; // docs/public 未构建
  }
  server = http.createServer(async (req, res) => {
    try {
      let p = decodeURIComponent(new URL(req.url ?? "/", "http://localhost").pathname);
      if (p === BASE_PATH) {
        res.writeHead(301, { Location: BASE_PATH + "/" });
        res.end();
        return;
      }
      if (p.startsWith(BASE_PATH + "/")) p = p.slice(BASE_PATH.length);
      let fp = path.join(PUBLIC_DIR, p);
      const info = await stat(fp).catch(() => null);
      if (!info || info.isDirectory()) {
        if (!p.endsWith("/")) {
          res.writeHead(301, { Location: BASE_PATH + p + "/" });
          res.end();
          return;
        }
        fp = path.join(fp, "index.html");
      }
      const data = await readFile(fp);
      res.writeHead(200, { "Content-Type": CONTENT_TYPES[path.extname(fp)] ?? "application/octet-stream" });
      res.end(data);
    } catch {
      res.writeHead(404);
      res.end("not found");
    }
  });
  await new Promise<void>((resolve) => server!.listen(0, "127.0.0.1", resolve));
  const addr = server.address();
  base = `http://127.0.0.1:${typeof addr === "object" && addr ? addr.port : 0}${BASE_PATH}`;
  available = true;
});

test.afterAll(async () => {
  if (server) await new Promise<void>((resolve) => server!.close(() => resolve()));
});

function themeOf(page: Page) {
  return page.evaluate(() => ({
    dark: document.documentElement.classList.contains("dark"),
    stored: localStorage.getItem("gitdash-theme"),
  }));
}

test("主题切换：Light / Dark / System 并记住偏好", async ({ page }) => {
  test.skip(!available, "docs/public not built");
  await page.goto(base + "/");

  await page.selectOption("#theme-select", "dark");
  await expect.poll(async () => (await themeOf(page)).dark).toBe(true);
  expect((await themeOf(page)).stored).toBe("dark");

  await page.selectOption("#theme-select", "light");
  await expect.poll(async () => (await themeOf(page)).dark).toBe(false);
  expect((await themeOf(page)).stored).toBe("light");

  // System 跟随 prefers-color-scheme
  await page.emulateMedia({ colorScheme: "dark" });
  await page.selectOption("#theme-select", "system");
  await expect.poll(async () => (await themeOf(page)).dark).toBe(true);
  expect((await themeOf(page)).stored).toBe("system");

  // 偏好持久化：刷新后仍为 System 且跟随系统
  await page.reload();
  await expect(page.locator("#theme-select")).toHaveValue("system");
  await expect.poll(async () => (await themeOf(page)).dark).toBe(true);
});

test("语言切换：跳到对应语言且保留当前页面", async ({ page }) => {
  test.skip(!available, "docs/public not built");
  await page.goto(base + "/getting-started/register/");

  await page.selectOption("#lang-select", { label: "中文" });
  await page.waitForURL(base + "/zh-cn/getting-started/register/");
  await expect(page.locator("h1")).toHaveText("注册与登录");
  // 页头“快速开始”链接也要带语言前缀
  await expect(page.locator('a.header-link[href$="/zh-cn/getting-started/"]')).toBeVisible();

  // 切回英文
  await page.selectOption("#lang-select", { label: "English" });
  await page.waitForURL(base + "/getting-started/register/");
  await expect(page.locator("h1")).toHaveText("Sign up and sign in");
});
