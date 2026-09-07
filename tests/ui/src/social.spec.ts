import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy Star / Watch / Fork", () => {
  test("Star / Unstar 切换并更新计数", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    await page.getByRole("link", { name: repoName, exact: true }).click();
    const starBtn = page.getByRole("button", { name: /^Star/ });
    await starBtn.click();
    await expect(page.getByRole("button", { name: /^Starred/ })).toBeVisible();
    // 计数 +1
    await expect(page.getByRole("button", { name: /^Starred/ })).toHaveText(/1/);

    await page.getByRole("button", { name: /^Starred/ }).click();
    await expect(page.getByRole("button", { name: /^Star/ })).toBeVisible();
  });

  test("Watch 状态切换并进入 Watching 列表", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    await page.getByRole("link", { name: repoName, exact: true }).click();
    // 建仓者自动关注自己：初始应为 Watching 1
    const watchingBtn = page.getByRole("button", { name: "Watching 1" });
    await expect(watchingBtn).toBeVisible();

    // 取关 → Watch 0；再关注 → Watching 1
    await watchingBtn.click();
    await expect(page.getByRole("button", { name: "Watch 0" })).toBeVisible();
    await page.getByRole("button", { name: "Watch 0" }).click();
    await expect(page.getByRole("button", { name: "Watching 1" })).toBeVisible();

    await page.getByRole("link", { name: "Repositories" }).click();
    await page.getByRole("tab", { name: "Watching" }).click();
    await expect(page.getByRole("link", { name: repoName, exact: true })).toBeVisible();
  });

  test("Fork 生成自己的副本（需换个名字，同名 fork 被拒绝）", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    await page.getByRole("link", { name: repoName, exact: true }).click();
    await page.getByRole("button", { name: "Fork" }).click();
    const dialog = page.getByRole("dialog");
    // 同名 fork 会 409（与 GitHub 一致），改一个新名字
    await dialog.locator("#fork-name").fill(`${repoName}-fork`);
    await dialog.getByRole("button", { name: "Fork" }).click();
    await expect(toast(page)).toBeVisible();

    // 跳转到 fork 出来的仓库
    await expect(
      page.getByRole("heading", { name: `${username}/${repoName}-fork` }),
    ).toBeVisible();
    await expect(page.getByText("forked from")).toBeVisible();
  });
});
