import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import {
  createRepoViaUi,
  openCollabsDialog,
  openWebhooksDialog,
  registerViaUi,
  toast,
} from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

/** 在独立 context 里再注册一个用户（协作者），返回其用户名与清理函数 */
async function registerSecondUser(
  baseURL: string,
  browser: {
    newContext: (opts?: object) => Promise<{
      newPage: () => Promise<Page>;
      close: () => Promise<void>;
    }>;
  },
) {
  const ctx = await browser.newContext({ locale: "en-US", baseURL });
  const page = await ctx.newPage();
  const { username } = await registerViaUi(page);
  return { username, page, close: () => ctx.close() };
}

test.describe("@happy 协作者", () => {
  test("添加协作者（读写）后对方仓库列表出现该仓库", async ({ page, browser, inst }) => {
    const { username: _owner } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    const b = await registerSecondUser(inst.baseURL, browser);

    await openCollabsDialog(page, repoName);
    const dialog = page.getByRole("dialog");
    await dialog.locator("#collab-username").fill(b.username);
    await dialog.getByRole("button", { name: "Add collaborator" }).click();
    await expect(toast(page)).toBeVisible();
    await expect(dialog.getByText(b.username)).toBeVisible();
    await page.keyboard.press("Escape");

    // 对方登录态下仓库列表出现共享仓库
    await b.page.goto("/");
    await expect(b.page.getByRole("link", { name: repoName, exact: true })).toBeVisible();
    await b.close();
  });
});

test.describe("@happy Webhook", () => {
  test("添加后列表可见", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    await openWebhooksDialog(page, repoName);
    const dialog = page.getByRole("dialog");
    await dialog.locator("#wh-url").fill("http://127.0.0.1:19999/hook");
    await dialog.getByRole("button", { name: "Add webhook" }).click();
    await expect(toast(page)).toBeVisible();
    await expect(dialog.getByText("http://127.0.0.1:19999/hook")).toBeVisible();
  });
});

test.describe("@happy Releases", () => {
  test("创建 Release 并出现在列表", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();

    await page.getByRole("tab", { name: "Releases" }).click();
    await page.getByRole("button", { name: "New release" }).first().click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("#release-tag").fill("v1.0.0");
    await dialog.locator("#release-title").fill("First release");
    await dialog.locator("#release-body").fill("notes");
    await dialog.getByRole("button", { name: "Create" }).click();

    await expect(page.getByText("v1.0.0").first()).toBeVisible();
  });
});
