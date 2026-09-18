import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

async function openSettings(page: import("@playwright/test").Page) {
  await page.getByRole("tab", { name: "Settings" }).click();
}

test.describe("@happy 仓库设置", () => {
  test("描述 / 标签 / Issues / 模板 通过界面保存", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await openSettings(page);

    // 描述
    const desc = "settings e2e description";
    const descInput = page.getByPlaceholder("Short description of this repository");
    await descInput.fill(desc);
    await descInput.locator("xpath=..").getByRole("button", { name: "Save" }).click();
    await expect(toast(page)).toContainText("Repository description updated");

    // 标签
    const topicsInput = page.getByPlaceholder("go, web, cli");
    await topicsInput.fill("e2e, test");
    await topicsInput.locator("xpath=..").getByRole("button", { name: "Save" }).click();
    await expect(toast(page)).toContainText("Tags updated");

    // Issues 开 → 关
    await page.getByRole("button", { name: "Disable issues" }).click();
    await expect(toast(page)).toContainText("Issues disabled");
    await page.getByRole("button", { name: "Enable issues" }).click();
    await expect(toast(page)).toContainText("Issues enabled");

    // 模板 开 → 关
    await page.getByRole("button", { name: "Make template" }).click();
    await expect(toast(page)).toContainText("Repository is now a template");
    await page.getByRole("button", { name: "Remove template" }).click();
    await expect(toast(page)).toContainText("Repository is no longer a template");
  });

  test("环境变量与密钥可通过界面新增", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await openSettings(page);

    // 环境变量
    await page.getByPlaceholder("KEY").fill("E2E_VAR");
    await page.getByPlaceholder("VALUE").fill("e2e-value");
    await page.getByPlaceholder("VALUE").locator("xpath=../..").getByRole("button", { name: "Save" }).click();
    await expect(toast(page)).toContainText("Environment variable E2E_VAR saved");
    await expect(page.locator("code", { hasText: "E2E_VAR" })).toBeVisible();

    // 密钥（值保存后只显示掩码）
    await page.getByPlaceholder("MY_SECRET").fill("E2E_SECRET");
    await page.getByPlaceholder("••••••").fill("s3cr3t");
    await page.getByPlaceholder("••••••").locator("xpath=../..").getByRole("button", { name: "Save" }).click();
    await expect(toast(page)).toContainText("Secret E2E_SECRET saved");
    await expect(page.locator("code", { hasText: "E2E_SECRET" })).toBeVisible();
  });

  test("分支保护规则可保存", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await openSettings(page);

    await page.getByRole("button", { name: "Save rule" }).click();
    await expect(toast(page)).toContainText("Protection rule for main saved");
    await expect(page.locator("code", { hasText: "main" }).first()).toBeVisible();
  });

  test("Run git gc 确认后执行", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await openSettings(page);

    await page.getByRole("button", { name: "Run git gc" }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button").last().click();
    await expect(toast(page)).toContainText("git gc finished");
  });
});
