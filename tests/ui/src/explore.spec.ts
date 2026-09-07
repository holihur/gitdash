import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy 探索 / 可见性", () => {
  test("设为公开后出现在 Explore 列表，并可被全局搜索命中", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    // settings tab：设为公开
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await page.getByRole("tab", { name: "Settings" }).click();
    await page.getByRole("button", { name: "Make public" }).click();
    await expect(toast(page)).toBeVisible();

    // Explore 列表出现该仓库（卡片链接文案是 owner/name）
    await page.getByRole("link", { name: "Explore" }).click();
    await expect(page.getByRole("heading", { name: "Explore" })).toBeVisible();
    await expect(
      page.getByRole("link", { name: `${username}/${repoName}` }),
    ).toBeVisible();

    // 全局搜索命中
    const search = page.getByPlaceholder("Search repositories, issues, users…");
    await search.fill(repoName);
    await expect(page.getByText(`${username}/${repoName}`).first()).toBeVisible();
  });

  test("私有仓库不出现在 Explore", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" }); // 默认私有

    await page.getByRole("link", { name: "Explore" }).click();
    // 会话级共享实例里可能有其他测试的公开仓库，这里只断言自己的私有仓库不在列
    await expect(page.getByRole("heading", { name: "Explore" })).toBeVisible();
    expect(
      await page.getByRole("link", { name: `${username}/${repoName}` }).count(),
    ).toBe(0);
  });
});
