import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, deleteRepoViaUi, registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

async function waitToastsGone(page: import("@playwright/test").Page) {
  for (let i = 0; i < 60; i++) {
    if ((await page.locator("[data-sonner-toast]").count()) === 0) return;
    await page.waitForTimeout(500);
  }
}

/** 点击某行（以 <code> 文本定位）右侧的删除按钮。 */
function rowDelete(page: import("@playwright/test").Page, text: string) {
  return page.locator("code", { hasText: text }).locator("xpath=../..").getByRole("button");
}

test.describe("@happy 仓库设置删除", () => {
  test("环境变量 / 密钥 / 分支保护 / 入站 webhook / 仓库 删除", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await page.getByRole("tab", { name: "Settings" }).click();

    // 环境变量：新增后删除
    await page.getByPlaceholder("KEY").fill("DEL_VAR");
    await page.getByPlaceholder("VALUE").fill("v");
    await page.getByPlaceholder("VALUE").locator("xpath=../..").getByRole("button", { name: "Save" }).click();
    await expect(toast(page)).toContainText("Environment variable DEL_VAR saved");
    await waitToastsGone(page);
    await rowDelete(page, "DEL_VAR").click();
    await expect(toast(page)).toContainText("Environment variable DEL_VAR removed");

    // 密钥：新增后删除
    await waitToastsGone(page);
    await page.getByPlaceholder("MY_SECRET").fill("DEL_SECRET");
    await page.getByPlaceholder("••••••").fill("s3cr3t");
    await page.getByPlaceholder("••••••").locator("xpath=../..").getByRole("button", { name: "Save" }).click();
    await expect(toast(page)).toContainText("Secret DEL_SECRET saved");
    await waitToastsGone(page);
    await rowDelete(page, "DEL_SECRET").click();
    await expect(toast(page)).toContainText("Secret DEL_SECRET removed");

    // 分支保护：保存后删除
    await waitToastsGone(page);
    await page.getByRole("button", { name: "Save rule" }).click();
    await expect(toast(page)).toContainText("Protection rule for main saved");
    await waitToastsGone(page);
    await rowDelete(page, "main").click();
    await expect(toast(page)).toContainText("Protection rule for main removed");

    // 入站 webhook：启用后删除
    await waitToastsGone(page);
    await page.getByRole("button", { name: "Enable webhook" }).click();
    await expect(toast(page)).toContainText("Incoming webhook");
    await waitToastsGone(page);
    await page.getByRole("button", { name: "Delete", exact: true }).click();
    await expect(toast(page)).toContainText("Incoming webhook deleted");

    // 仓库删除（确认后回列表）
    await page.getByRole("link", { name: "Repositories" }).click();
    await waitToastsGone(page);
    await deleteRepoViaUi(page, repoName);
    await expect(page.getByRole("link", { name: repoName, exact: true })).toHaveCount(0);
  });
});
