import { test, expect, hasInstanceSource, type Page } from "./fixtures/browser";

/**
 * 管理台（/admin）E2E：登录、左侧菜单路由绑定、深链、强制 MFA 开关。
 * 通过真实 UI 驱动，不发 API 请求（登录也走管理台表单）。
 */

const ADMIN_USER = process.env.GITDASH_ADMIN_USER ?? "gitdash-admin";
const ADMIN_PASS = process.env.GITDASH_ADMIN_PASSWORD ?? "admin-test-pass-123456";

test.skip(!hasInstanceSource(), "UI tests need GITDASH_BIN or GITDASH_UI_URL");

async function adminLogin(page: Page) {
  await page.goto("/admin");
  const user = page.locator("#adm-u");
  if (await user.count()) {
    await user.fill(ADMIN_USER);
    await page.locator("#adm-p").fill(ADMIN_PASS);
    await page.getByRole("button", { name: "Sign in" }).click();
  }
  // 未启用管理面板：跳过（对齐 pytest admin fixture 的语义）
  if (await page.getByText("Admin panel is not enabled").count()) {
    test.skip(true, "admin panel disabled on this instance");
  }
  await expect(page.getByRole("heading", { name: "gitdash Admin" })).toBeVisible();
}

const SECTIONS: [string, string][] = [
  ["Authentication", "/admin/authentication"],
  ["Content", "/admin/content"],
  ["Moderation", "/admin/moderation"],
  ["System", "/admin/system"],
  ["General", "/admin"],
];

test.describe("Admin panel", () => {
  test("左侧菜单切换：URL 绑定且内容非空（回归白屏）", async ({ page }) => {
    await adminLogin(page);
    for (const [name, path] of SECTIONS) {
      await page.locator('[role="tab"]', { hasText: name }).first().click();
      await expect(page).toHaveURL(new RegExp(`${path.replace(/\//g, "\\/")}$`));
      // 选中态跟随
      await expect(page.locator('[role="tab"][aria-selected="true"]').first()).toContainText(name);
      // 内容区非空（白屏回归）
      const main = page.locator("main");
      await expect(main).toBeVisible();
      expect(((await main.innerText()) ?? "").trim().length).toBeGreaterThan(0);
    }
  });

  test("深链直接进入指定分区", async ({ page }) => {
    await adminLogin(page);
    await page.goto("/admin/system");
    await expect(page.locator('[role="tab"][aria-selected="true"]').first()).toContainText("System");
    await expect(page.getByText("IP blacklist")).toBeVisible();
  });

  test("未知路径回退到 General（不白屏）", async ({ page }) => {
    await adminLogin(page);
    await page.goto("/admin/does-not-exist/deep");
    await expect(page.locator('[role="tab"][aria-selected="true"]').first()).toContainText("General");
    await expect(page.locator("main")).toBeVisible();
  });

  test("可开启强制两步验证并持久化", async ({ page }) => {
    await adminLogin(page);
    const checkbox = page.getByRole("checkbox", {
      name: /Require two-factor authentication/i,
    });
    await expect(checkbox).toBeVisible();
    const wasChecked = await checkbox.isChecked();
    if (!wasChecked) await checkbox.check();
    await page.getByRole("button", { name: /^Save$/ }).first().click();

    await page.reload();
    const after = page.getByRole("checkbox", { name: /Require two-factor authentication/i });
    await expect(after).toBeChecked();

    // 复原，避免影响同实例的其它用例
    if (!wasChecked) {
      await after.uncheck();
      await page.getByRole("button", { name: /^Save$/ }).first().click();
    }
  });
});
