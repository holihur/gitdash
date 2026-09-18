import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, registerViaUi } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy 导航 / 只读端点", () => {
  test("Explore 全局搜索 + 代码搜索", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: "Explore" }).click();

    const globalResp = page.waitForResponse(
      (r) => r.url().includes("/api/search?") && r.status() === 200,
    );
    const codeResp = page.waitForResponse(
      (r) => r.url().includes("/api/search/code") && r.status() === 200,
    );
    await page.getByPlaceholder("Search repositories, issues, users, code…").fill(repoName);
    await Promise.all([globalResp, codeResp]);
  });

  test("Repos 页 Starred 标签", async ({ page }) => {
    await registerViaUi(page);
    await page.getByRole("tab", { name: "Starred" }).click();
    await expect(page.getByText("No starred repositories")).toBeVisible();
  });

  test("Packages 页面加载", async ({ page }) => {
    await registerViaUi(page);
    await page.goto("/packages");
    await expect(page.getByRole("heading", { name: "Package Registry" })).toBeVisible();
  });

  test("仓库标签页与提交 diff", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();

    await page.getByRole("tab", { name: "Commits" }).click();
    const diffResp = page.waitForResponse(
      (r) => /\/commits\/[0-9a-f]+\/diff/.test(r.url()) && r.status() === 200,
    );
    await page.getByTitle("View diff").first().click();
    await diffResp;

    for (const tab of ["Pipeline", "Copilot", "Projects"]) {
      await page.getByRole("tab", { name: tab }).click();
      await expect(page.getByRole("tab", { name: tab })).toHaveAttribute("aria-selected", "true");
    }
  });
});
