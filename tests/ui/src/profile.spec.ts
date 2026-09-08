import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { loginViaUi, registerViaUi, signOutViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("@happy 个人资料 / 安全", () => {
  test("修改密码后用新密码重新登录", async ({ page }) => {
    const { username, password } = await registerViaUi(page);

    await page.getByRole("link", { name: username }).click(); // /profile
    await expect(page.getByRole("heading", { name: "Profile" })).toBeVisible();

    const next = password + "-new";
    await page.locator("#pw-current").fill(password);
    await page.locator("#pw-new").fill(next);
    await page.locator("#pw-confirm").fill(next);
    await page.getByRole("button", { name: "Change password" }).click();
    await expect(toast(page)).toBeVisible();

    // 旧密码被拒（401 只弹错误 toast，留在登录页，不再整页跳转）
    await signOutViaUi(page);
    await loginViaUi(page, username, password);
    await expect(toast(page, "error")).toBeVisible();
    await expect(page.locator("#username")).toBeVisible();

    // 重新登录后回到登出前的 /profile
    await loginViaUi(page, username, next);
    await expect(page.getByRole("heading", { name: "Profile" })).toBeVisible();
  });

  test("设置邮箱并保存（未配置 SMTP 时直接标记已验证）", async ({ page }) => {
    const { username } = await registerViaUi(page);

    await page.getByRole("link", { name: username }).click();
    await page.locator("#profile-email").fill(`${username}@example.com`);
    await page.getByRole("button", { name: "Save" }).click();
    await expect(toast(page)).toBeVisible();
    await expect(page.getByText("Email verified")).toBeVisible();
  });
});
