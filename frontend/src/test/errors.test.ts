import { describe, expect, it } from "vitest";
import { apiErrorMsg } from "@/lib/errors";
import { ApiError } from "@/lib/api";

const t = (key: string) => (key === "errors.repo_not_found" ? "仓库不存在" : undefined);

describe("apiErrorMsg", () => {
  it("优先按后端错误码 i18n", () => {
    const err = new ApiError(404, "repo not found", "repo_not_found");
    expect(apiErrorMsg(t, err)).toBe("仓库不存在");
  });

  it("无匹配错误码时回退到后端 message", () => {
    const err = new ApiError(500, "boom", "internal_error");
    expect(apiErrorMsg(t, err)).toBe("boom");
  });

  it("非 ApiError 用 message", () => {
    expect(apiErrorMsg(t, new Error("net down"))).toBe("net down");
  });

  it("使用 fallbackKey 兜底", () => {
    expect(apiErrorMsg(t, undefined, "common.copyFailed")).toBe("common.copyFailed");
  });
});
