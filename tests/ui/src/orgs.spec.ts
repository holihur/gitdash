import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

async function registerSecondUser(
  baseURL: string,
  browser: {
    newContext: (opts?: object) => Promise<{
      newPage: () => Promise<Page>;
      close: () => Promise<void>;
    }>;
  },
) {
  const ctx = await browser.newContext({ locale: "en-US", baseURL });
  const page = await ctx.newPage();
  const { username } = await registerViaUi(page);
  return { username, page, close: () => ctx.close() };
}

function randOrg(): string {
  return `e2e-org-${Math.random().toString(36).slice(2, 8)}`;
}

test.describe("@happy 组织", () => {
  test("创建/关注/成员/删除 组织", async ({ page, browser, inst }) => {
    await registerViaUi(page);

    // 创建组织
    await page.goto("/orgs");
    await page.getByRole("button", { name: "New organization" }).click();
    const org = randOrg();
    await page.locator("#org-name").fill(org);
    await page.locator("#org-display").fill("E2E Org");
    await page.getByRole("dialog").getByRole("button", { name: "Create" }).click();
    await expect(toast(page)).toContainText(org);

    // 组织页（GET profile）
    await page.goto(`/orgs/${org}`);
    await expect(page.getByText(org).first()).toBeVisible();

    // 另一个用户关注 / 取关（GET followers + POST/DELETE follow）
    const b = await registerSecondUser(inst.baseURL, browser);
    await b.page.goto(`/orgs/${org}`);
    await b.page.getByRole("button", { name: "Follow", exact: true }).click();
    await expect(b.page.getByRole("button", { name: "Unfollow", exact: true })).toBeVisible();
    await b.page.getByRole("button", { name: "Unfollow", exact: true }).click();
    await expect(b.page.getByRole("button", { name: "Follow", exact: true })).toBeVisible();

    // 成员：加入再移除
    await page.getByRole("button", { name: /Members/ }).click();
    await page.getByPlaceholder("Username").fill(b.username);
    await page.getByRole("button", { name: "Add", exact: true }).click();
    await expect(toast(page)).toContainText(`Member ${b.username} added`);
    // 等 toast 消失，避免其覆盖成员行上的删除按钮
    await expect(toast(page)).toBeHidden({ timeout: 15000 });
    const delResp = page.waitForResponse(
      (r) => r.request().method() === "DELETE" && r.url().includes("/members/"),
    );
    await page
      .getByRole("link")
      .filter({ hasText: b.username })
      .locator("xpath=following-sibling::button")
      .click();
    expect((await delResp).status()).toBe(204);
    await expect(toast(page)).toContainText(`Member ${b.username} removed`);

    // 关注者标签页（GET followers）
    await page.getByRole("button", { name: /Followers/ }).click();
    await expect(page.getByText("No followers yet")).toBeVisible();

    // 删除组织
    await page.getByRole("button", { name: "Delete organization" }).click();
    await page.getByRole("alertdialog").getByRole("button").last().click();
    await expect(toast(page)).toContainText("deleted");

    await b.close();
  });

  test("用户页：关注 / 取关与粉丝列表", async ({ page, browser, inst }) => {
    const { username: me } = await registerViaUi(page);
    const b = await registerSecondUser(inst.baseURL, browser);

    // 从 B 访问 A 的公开用户页（GET /users/{username}）
    await b.page.goto(`/users/${me}`);
    await expect(b.page.getByText(me).first()).toBeVisible();

    // 关注 A（POST /users/{}/follow）→ 取关（DELETE）
    await b.page.getByRole("button", { name: "Follow", exact: true }).click();
    await expect(b.page.getByRole("button", { name: "Unfollow", exact: true })).toBeVisible();
    await b.page.getByRole("button", { name: "Unfollow", exact: true }).click();
    await expect(b.page.getByRole("button", { name: "Follow", exact: true })).toBeVisible();

    // 粉丝 / 关注列表（GET followers / following）
    await b.page.getByRole("button", { name: /followers/i }).click();
    await expect(b.page.getByText("No followers yet")).toBeVisible();
    await b.page.getByRole("button", { name: /following/i }).click();
    await expect(b.page.getByText("Not following anyone yet")).toBeVisible();

    await b.close();
  });
});
