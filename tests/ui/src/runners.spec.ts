import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { registerViaUi } from "./helpers/ui";

test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

test.describe("Runners（自托管 CI agent）", () => {
  test("个人设置页可见 Runners 卡片：空列表 + 签发一次性注册 token", async ({ page }) => {
    const { username } = await registerViaUi(page);

    // 导航栏用户名 → /profile
    await page.getByRole("link", { name: username }).click();
    await expect(page.getByText("Self-hosted CI agents connect to this server")).toBeVisible();
    await expect(page.getByText("No runners registered yet.")).toBeVisible();

    // 签发注册 token：一次性展示（仅此一次）
    await page.getByRole("button", { name: "Issue registration token" }).click();
    await expect(page.getByText("One-time registration token")).toBeVisible();
    const tokenCode = page.locator("code.block");
    await expect(tokenCode).toBeVisible();
    const token = (await tokenCode.textContent()) ?? "";
    expect(token.length).toBeGreaterThanOrEqual(32);
  });

  test("未登录时无法签发 runner 注册 token（401）", async ({ page }) => {
    await page.goto("/");
    const resp = await page.request.post("/api/runners/registration-token", {
      data: { scope: "user" },
    });
    expect(resp.status()).toBe(401);
  });
});
