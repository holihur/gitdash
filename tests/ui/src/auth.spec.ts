import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { loginViaUi, registerViaUi, signOutViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy 认证 / 会话", () => {
  test("注册即自动登录，进入仓库页", async ({ page }) => {
    const { username } = await registerViaUi(page);
    // 顶栏显示当前用户
    await expect(page.getByRole("link", { name: username })).toBeVisible();
  });

  test("登出后回到登录页，可重新登录", async ({ page }) => {
    const { username, password } = await registerViaUi(page);

    await signOutViaUi(page);
    await loginViaUi(page, username, password);
    await expect(page.getByRole("heading", { name: "Repositories" })).toBeVisible();
  });

  test("错误密码被拒绝（提示错误）", async ({ page }) => {
    const { username, password } = await registerViaUi(page);
    await signOutViaUi(page);

    await loginViaUi(page, username, password + "x");
    await expect(toast(page)).toBeVisible();
    // 仍停留在登录页
    await expect(page.locator("#username")).toBeVisible();
  });

  test("登录页展示 Swagger UI 入口", async ({ page }) => {
    await page.goto("/");
    const link = page.getByRole("link", { name: "API Docs (Swagger)" });
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute("href", "/api/swagger/");
  });
});
