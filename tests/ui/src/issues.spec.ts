import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, openRepoViaUi, registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy Issue", () => {
  test("创建 Issue 后出现在列表中", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await openRepoViaUi(page, username, repoName);

    await page.getByRole("tab", { name: "Issues" }).click();
    await expect(page.getByRole("button", { name: "New issue" })).toBeVisible();

    const title = "ui e2e issue";
    await page.getByRole("button", { name: "New issue" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.locator("#issue-title").fill(title);
    await dialog.locator("#issue-body").fill("reported by the ui issue test");
    await dialog.getByRole("button", { name: "New issue" }).click();

    await expect(page.getByText(title).first()).toBeVisible();
    await expect(toast(page)).toBeVisible();
  });

  test("空标题不可提交（按钮禁用）", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await openRepoViaUi(page, username, repoName);

    await page.getByRole("tab", { name: "Issues" }).click();
    await page.getByRole("button", { name: "New issue" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("button", { name: "New issue" })).toBeDisabled();
  });
});
