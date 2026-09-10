import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, expect } from "./fixtures/browser";
import { hasInstanceSource } from "./fixtures/browser";
import { registerViaUi, gotoNav, toast } from "./helpers/ui";

// 对齐 tests/conftest.py：未配置实例来源（GITDASH_BIN / GITDASH_UI_URL）时整组 skip
test.skip(
  !hasInstanceSource(),
  "UI tests need a server instance: set GITDASH_BIN or GITDASH_UI_URL",
);

/** 生成一对 ed25519 密钥，返回公钥文本（与 scripts/e2e.sh 同款做法） */
function genSshKey(): string {
  const dir = mkdtempSync(path.join(tmpdir(), "ui-key-"));
  try {
    execFileSync("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-f", path.join(dir, "k")]);
    return execFileSync("cat", [path.join(dir, "k.pub")], { encoding: "utf8" }).trim();
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

test.describe("@happy SSH 密钥 / 访问令牌", () => {
  test("添加 SSH 公钥并显示指纹，可删除", async ({ page }) => {
    await registerViaUi(page);
    await gotoNav(page, "SSH Keys");
    await expect(page.getByRole("heading", { name: "SSH Keys" })).toBeVisible();

    await page.getByRole("button", { name: "Add key" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("#key-name").fill("ui-test-key");
    await dialog.locator("#key-pub").fill(genSshKey());
    // 对话框内提交按钮文案是 common.add = "Add"
    await dialog.getByRole("button", { name: "Add", exact: true }).click();
    await expect(toast(page)).toBeVisible();

    // 列表出现指纹
    await expect(page.getByText("SHA256:").first()).toBeVisible();

    // 删除（删除按钮是垃圾桶图标，确认对话框）
    await page.getByRole("button", { name: "Delete key ui-test-key" }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
    await expect(toast(page)).toBeVisible();
  });

  test("创建 PAT：令牌只展示一次，列表可见", async ({ page }) => {
    await registerViaUi(page);
    await gotoNav(page, "SSH Keys");
    await page.getByRole("tab", { name: "Personal access tokens" }).click();

    await page.getByRole("button", { name: "Create token" }).first().click();
    const dialog = page.getByRole("dialog");
    await dialog.locator("#pat-name").fill("ui-ci-token");
    await dialog.getByRole("button", { name: "Create token" }).click();

    // 一次性展示令牌
    await expect(page.getByText("Copy this token now")).toBeVisible();
    // Save 按钮不稳定（toast 覆盖层导致），用 Escape 关闭
    await page.keyboard.press("Escape");

    // 列表出现
    await expect(page.getByText("ui-ci-token")).toBeVisible();
  });
});
