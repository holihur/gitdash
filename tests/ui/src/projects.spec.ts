import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { createRepoViaUi, registerViaUi, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

async function waitToastsGone(page: import("@playwright/test").Page) {
  for (let i = 0; i < 60; i++) {
    if ((await page.locator("[data-sonner-toast]").count()) === 0) return;
    await page.waitForTimeout(500);
  }
}

test.describe("@happy 项目看板", () => {
  test("创建项目 / 列 / 泳道 / 卡片 → 重命名 → 删除", async ({ page }) => {
    await registerViaUi(page);
    const repoName = await createRepoViaUi(page, { template: "readme" });
    await page.getByRole("link", { name: repoName, exact: true }).click();
    await page.getByRole("tab", { name: "Projects" }).click();

    // 创建项目（POST /projects，列表 GET /projects）
    await page.getByRole("button", { name: "New project" }).first().click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#proj-name").fill("E2E Board");
    await dlg.getByRole("button", { name: "New project" }).click();
    await expect(toast(page)).toContainText("Project created");

    // 打开看板（GET /projects/{}/board）
    await page.getByText("E2E Board").first().click();
    await expect(page.getByRole("button", { name: "Add column" })).toBeVisible();

    // 添加列（POST /projects/{}/columns）
    await waitToastsGone(page);
    await page.getByRole("button", { name: "Add column" }).click();
    const colDlg = page.getByRole("dialog");
    await colDlg.getByPlaceholder("e.g. To Do").fill("To Do");
    await colDlg.getByRole("button", { name: "Add column" }).click();
    await expect(toast(page)).toContainText("Column created");

    // 添加泳道（POST /projects/{}/swimlanes）
    await waitToastsGone(page);
    await page.getByRole("button", { name: "Add swimlane" }).click();
    const laneDlg = page.getByRole("dialog");
    await laneDlg.getByPlaceholder("e.g. Default").fill("Default");
    await laneDlg.getByRole("button", { name: "Add swimlane" }).click();
    await expect(toast(page)).toContainText("Swimlane created");

    // 添加卡片（POST /projects/{}/cards）
    await waitToastsGone(page);
    await page.getByRole("button", { name: "Add card" }).first().click();
    const cardDlg = page.getByRole("dialog");
    await cardDlg.locator("#card-title").fill("First card");
    await cardDlg.getByRole("button", { name: "Add card" }).click();
    await expect(toast(page)).toContainText("Card created");

    // 卡片必须出现在看板上（回归：文本卡片曾被当成 issue #0 渲染，标题不显示）
    await waitToastsGone(page);
    await expect(page.getByText("First card")).toBeVisible();

    // 列表视图也能新建卡片（全局 Add card + 选择列/泳道）
    await page.getByRole("button", { name: "List" }).click();
    await page.getByRole("button", { name: "Add card" }).click();
    const listDlg = page.getByRole("dialog");
    await listDlg.locator("#card-title").fill("List card");
    await listDlg.getByRole("button", { name: "Add card" }).click();
    await expect(toast(page)).toContainText("Card created");
    await waitToastsGone(page);
    await expect(page.getByText("List card")).toBeVisible();

    // 重命名项目（PATCH /projects/{}）
    await waitToastsGone(page);
    await page.getByTitle("Rename project").click();
    await page.locator("input.h-8").fill("Renamed Board");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect(toast(page)).toContainText("Project updated");

    // 删除项目（DELETE /projects/{}）
    await waitToastsGone(page);
    await page.getByRole("button", { name: "All projects" }).click();
    await page.getByTitle("Delete project").click();
    await page.getByRole("alertdialog").getByRole("button").last().click();
    await expect(toast(page)).toContainText("deleted");
  });
});
