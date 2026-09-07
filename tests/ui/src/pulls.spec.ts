import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import {
  addFileViaUi,
  createBranchViaUi,
  createRepoViaUi,
  registerViaUi,
  toast,
} from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy Pull Request", () => {
  test("建分支 → 网页提交文件 → 建 PR → 合并", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();

    // 1) 建分支
    await createBranchViaUi(page, "feature-1");

    // 2) 在 feature-1 上网页提交一个文件
    await addFileViaUi(page, {
      branch: "feature-1",
      path: "docs/hello.md",
      content: "# hello from ui test",
    });

    // 3) 建 PR
    await page.getByRole("tab", { name: "Pull Requests" }).click();
    await page.getByRole("button", { name: "New pull request" }).first().click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("#pr-title").fill("add hello doc");
    await dialog.locator("#pr-source").selectOption("feature-1");
    await dialog.locator("#pr-target").selectOption("main");
    await dialog.getByRole("button", { name: "New pull request" }).click();
    await expect(page.getByText("add hello doc").first()).toBeVisible();

    // 4) 合并
    await page.getByRole("button", { name: "Merge", exact: true }).click();
    await expect(page.getByText("merged by").first()).toBeVisible();
    await expect(toast(page)).toBeVisible();
  });

  test("同分支 PR 被拒绝（源/目标相同）", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();

    await page.getByRole("tab", { name: "Pull Requests" }).click();
    await page.getByRole("button", { name: "New pull request" }).first().click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("#pr-title").fill("same branch pr");
    // source 与 target 都是 main → 提交按钮禁用
    await expect(dialog.getByRole("button", { name: "New pull request" })).toBeDisabled();
  });
});
