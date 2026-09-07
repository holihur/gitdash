import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy 仓库管理", () => {
  test("创建仓库（README 模板），卡片与 clone 命令出现", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const desc = "created by e2e ui repo test";
    const repoName = await createRepoViaUi(page, {
      description: desc,
      template: "readme",
    });

    // 卡片出现：名称链接 + 描述 + clone 命令
    await expect(page.getByRole("link", { name: repoName, exact: true })).toBeVisible();
    await expect(page.getByText(desc)).toBeVisible();
    await expect(page.getByText("git clone ssh://")).toBeVisible();
    await expect(page.locator("code", { hasText: `${username}/${repoName}` })).toBeVisible();
  });

  test("同名仓库被拒绝（错误提示）", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    // 再次创建同名
    await createRepoViaUi(page, { name: repoName });
    await expect(toast(page)).toBeVisible();
  });

  test("非法仓库名被拒绝（错误提示）", async ({ page }) => {
    await registerViaUi(page);

    await createRepoViaUi(page, { name: "-invalid-name" });
    await expect(toast(page)).toBeVisible();
  });
});
