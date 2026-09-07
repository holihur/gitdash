import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, openRepoViaUi, registerViaUi } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy 代码浏览", () => {
  test("仓库页展示 README.md 并可查看渲染内容", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    await openRepoViaUi(page, username, repoName);

    // code tab 默认激活，目录里应有模板生成的 README.md
    await expect(page.getByRole("tab", { name: "Code" })).toHaveAttribute("aria-selected", "true");
    const readmeLink = page.getByRole("cell").filter({ hasText: "README.md" });
    await expect(readmeLink).toBeVisible();

    // 打开 blob：README 内容为 "# <repoName>"，markdown 渲染成一级标题
    await readmeLink.click();
    await expect(
      page.getByRole("heading", { level: 1, name: repoName, exact: true }),
    ).toBeVisible();
  });

  test("Commits tab 展示初始提交", async ({ page }) => {
    const { username } = await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });

    await openRepoViaUi(page, username, repoName);
    await page.getByRole("tab", { name: "Commits" }).click();
    await expect(page.getByText("Initial commit").first()).toBeVisible();
  });
});
