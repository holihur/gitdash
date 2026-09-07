import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import {
  createRepoViaUi,
  openCollabsDialog,
  registerViaUi,
  toast,
} from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy 收件箱通知", () => {
  // 说明：当前产品提 issue 需要 write 权限（owner 会被排除出自己的通知），
  // 所以这里把第二用户加为 write 协作者来触发通知。
  test("关注者收到 issue 通知；操作者本人不收到自己的通知", async ({ page, browser, inst }) => {
    // owner：建公开仓库（自己自动关注）
    const { username: owner } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await page.getByRole("tab", { name: "Settings" }).click();
    await page.getByRole("button", { name: "Make public" }).click();
    await expect(toast(page)).toBeVisible();
    await page.getByRole("link", { name: "Repositories" }).click();

    // watcher：第二用户加为 write 协作者
    const ctxW = await browser.newContext({ locale: "en-US", baseURL: inst.baseURL });
    const wPage = await ctxW.newPage();
    const { username: watcher } = await registerViaUi(wPage);
    await openCollabsDialog(page, repoName);
    const cdialog = page.getByRole("dialog");
    await cdialog.locator("#collab-username").fill(watcher);
    await cdialog.getByRole("button", { name: "Add collaborator" }).click();
    await expect(toast(page)).toBeVisible();
    await page.keyboard.press("Escape");

    // watcher 从 Explore 进入仓库并关注
    await wPage.getByRole("link", { name: "Explore" }).click();
    await wPage.getByRole("link", { name: `${owner}/${repoName}` }).click();
    const watchBtn = wPage.getByRole("button", { name: /^Watch \d/ });
    await expect(watchBtn).toBeVisible();
    await watchBtn.click();
    await expect(wPage.getByRole("button", { name: "Watching 2" })).toBeVisible();

    // watcher 开 issue → owner（关注者）收到通知，操作者本人不收到
    const title = "inbox notification issue";
    await wPage.getByRole("tab", { name: "Issues" }).click();
    await wPage.getByRole("button", { name: "New issue" }).click();
    const dialog = wPage.getByRole("dialog");
    await dialog.locator("#issue-title").fill(title);
    await dialog.getByRole("button", { name: "New issue" }).click();
    await expect(wPage.getByText(title).first()).toBeVisible();

    // owner 的收件箱出现通知
    await page.getByRole("link", { name: "Inbox" }).click();
    await expect(page.getByRole("heading", { name: "Inbox" })).toBeVisible();
    await expect(page.getByText(`${owner}/${repoName}`).first()).toBeVisible();
    await expect(page.getByText(title)).toBeVisible();

    // watcher 自己的收件箱为空（actor 被排除）
    await wPage.getByRole("link", { name: "Inbox" }).click();
    await expect(wPage.getByText("Inbox is empty")).toBeVisible();

    // owner 全部已读
    await page.getByRole("button", { name: "Mark all as read" }).click();
    await expect(page.locator("span.bg-blue-500")).toHaveCount(0);

    await ctxW.close();
  });
});
