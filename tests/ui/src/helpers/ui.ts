import { expect, type Page } from "@playwright/test";
import { randRepo, randUser } from "../fixtures/browser";

/**
 * UI 操作助手：跨业务 spec 复用的页面动作（注册/建仓/登出等）。
 * 全部通过真实 UI 点击完成，不发 API 请求。
 */

export const DEFAULT_PASSWORD = "ui-pass-123456";

/** 通过登录页 Register tab 注册新用户，注册即自动登录进入仓库页 */
export async function registerViaUi(page: Page): Promise<{ username: string; password: string }> {
  const username = randUser();
  await page.goto("/");
  await expect(page.locator("#username")).toBeVisible();

  await page.getByRole("tab", { name: "Register" }).click();
  await page.locator("#username").fill(username);
  await page.locator("#password").fill(DEFAULT_PASSWORD);
  await page.getByRole("button", { name: "Register and sign in" }).click();

  await expect(page.getByRole("heading", { name: "Repositories" })).toBeVisible();
  return { username, password: DEFAULT_PASSWORD };
}

/** 登录页 Sign in */
export async function loginViaUi(page: Page, username: string, password: string) {
  await page.locator("#username").fill(username);
  await page.locator("#password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
}

/** 点击顶栏导航项：直接链接不可见（已折叠进「More」）时从下拉菜单打开。 */
export async function gotoNav(page: Page, name: string) {
  const link = page.getByRole("link", { name, exact: true });
  if ((await link.count()) > 0 && (await link.first().isVisible())) {
    await link.first().click();
    return;
  }
  await page.getByRole("button", { name: "More", exact: true }).click();
  await page.getByRole("menuitem", { name, exact: true }).click();
}

/** 通过仓库页 "New repository" 对话框创建仓库 */
export async function createRepoViaUi(
  page: Page,
  opts: { name?: string; description?: string; template?: "readme" } = {},
): Promise<string> {
  const name = opts.name ?? randRepo();
  await page.getByRole("button", { name: "New repository" }).click();
  await page.locator("#repo-name").fill(name);
  if (opts.description) await page.locator("#repo-desc").fill(opts.description);
  if (opts.template) await page.locator("#repo-template").selectOption(opts.template);
  await page.getByRole("button", { name: "Create", exact: true }).click();
  return name;
}

/** 打开仓库页（从仓库列表点击卡片链接） */
export async function openRepoViaUi(page: Page, owner: string, name: string) {
  await page.getByRole("link", { name: name, exact: true }).click();
  await expect(page.getByRole("heading", { name: `${owner}/${name}` })).toBeVisible();
}

/** 顶栏 "Sign out"，回到登录页 */
export async function signOutViaUi(page: Page) {
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.locator("#username")).toBeVisible();
}

/** sonner toast（取最新一条：多条堆叠时避免 strict mode 冲突） */
export function toast(page: Page, type?: "error" | "success" | "info" | "warning") {
  const base = type
    ? page.locator(`[data-sonner-toast][data-type="${type}"]`)
    : page.locator("[data-sonner-toast]");
  return base.last();
}

/** 打开仓库卡片右上角 ⋮ 菜单（协作者 / Webhook / 删除） */
export async function openRepoCardMenu(page: Page, repoName: string) {
  await page.getByRole("button", { name: `${repoName} More actions` }).click();
}

/** 通过仓库卡片 ⋮ 菜单打开协作者管理对话框 */
export async function openCollabsDialog(page: Page, repoName: string) {
  await openRepoCardMenu(page, repoName);
  await page.getByRole("menuitem", { name: "Collaborators" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
}

/** 通过仓库卡片 ⋮ 菜单打开 Webhook 管理对话框 */
export async function openWebhooksDialog(page: Page, repoName: string) {
  await openRepoCardMenu(page, repoName);
  await page.getByRole("menuitem", { name: "Webhooks" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
}

/** 通过仓库卡片 ⋮ 菜单删除仓库（含确认） */
export async function deleteRepoViaUi(page: Page, repoName: string) {
  await openRepoCardMenu(page, repoName);
  await page.getByRole("menuitem", { name: "Delete repository" }).click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Delete" }).click();
  await expect(toast(page)).toBeVisible();
}

/** 通过仓库页 "Branches & tags" 对话框创建分支 */
export async function createBranchViaUi(page: Page, branch: string) {
  await page.getByTitle("Branches & tags").click();
  const dialog = page.getByRole("dialog");
  await dialog.getByPlaceholder("feature/xxx").fill(branch);
  await dialog.getByRole("button", { name: "Create", exact: true }).first().click();
  await expect(toast(page)).toBeVisible();
  await page.keyboard.press("Escape");
}

/** 通过 code tab "New file" 对话框提交一个文件（网页端 commit） */
export async function addFileViaUi(
  page: Page,
  opts: { path: string; content: string; branch?: string },
) {
  await page.getByRole("button", { name: "New file" }).click();
  const dialog = page.getByRole("dialog");
  if (opts.branch) await dialog.locator("#fop-branch").selectOption(opts.branch);
  await dialog.locator("#fop-path").fill(opts.path);
  // 内容是 CodeMirror 编辑器
  await dialog.locator(".cm-content").click();
  await page.keyboard.type(opts.content);
  await dialog.getByRole("button", { name: "Commit changes" }).click();
  await expect(toast(page)).toBeVisible();
}
