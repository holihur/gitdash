import { test as base, expect, type Page } from "@playwright/test";
import {
  cleanupInstance,
  spawnOrUseExisting,
  type GitdashInstance,
} from "./server";

export { expect };
export { randRepo, randUser } from "./server";

/** 是否配置了实例来源（对齐 conftest.py：两者都未设置时整组 skip） */
export function hasInstanceSource(): boolean {
  return Boolean(
    (process.env.GITDASH_BIN ?? "").trim() || (process.env.GITDASH_UI_URL ?? "").trim(),
  );
}

type TestFixtures = {
  /** 每用例全新 context（cookie 隔离）的页面 */
  page: Page;
};

type WorkerFixtures = {
  /** 已就绪的 gitdash 实例（worker 级：同一 worker 内共享一个实例，worker 结束时清理） */
  inst: GitdashInstance;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
  inst: [
    async ({}, use) => {
      const inst = await spawnOrUseExisting();
      await use(inst);
      await cleanupInstance(inst);
    },
    { scope: "worker", auto: true },
  ],

  page: async ({ browser, inst }, use) => {
    // 全新 context：cookie/localStorage 完全隔离，用例之间零共享状态；
    // baseURL 指向实例（覆盖配置里的默认值）
    const ctx = await browser.newContext({ locale: "en-US", baseURL: inst.baseURL });
    const page = await ctx.newPage();
    await use(page);
    await ctx.close();
  },
});
