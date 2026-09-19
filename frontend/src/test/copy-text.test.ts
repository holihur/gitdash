import { afterEach, describe, expect, it, vi } from "vitest";
import { copyText } from "@/lib/utils";

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const originalExecCommand = (document as unknown as { execCommand?: unknown }).execCommand;

function setClipboard(impl?: (text: string) => Promise<void>) {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: impl ? { writeText: impl } : undefined,
  });
}

afterEach(() => {
  if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
  else Object.defineProperty(navigator, "clipboard", { configurable: true, value: undefined });
  (document as unknown as { execCommand?: unknown }).execCommand = originalExecCommand;
  vi.restoreAllMocks();
});

describe("copyText", () => {
  it("优先使用 Clipboard API", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard(writeText);
    const exec = vi.fn().mockReturnValue(true);
    (document as unknown as { execCommand: unknown }).execCommand = exec;

    await copyText("hello");

    expect(writeText).toHaveBeenCalledWith("hello");
    expect(exec).not.toHaveBeenCalled();
  });

  it("Clipboard API 被拒绝时回退到 execCommand", async () => {
    setClipboard(vi.fn().mockRejectedValue(new Error("denied")));
    const exec = vi.fn().mockReturnValue(true);
    (document as unknown as { execCommand: unknown }).execCommand = exec;

    await copyText("hello");

    expect(exec).toHaveBeenCalledWith("copy");
  });

  it("无 Clipboard API 时回退到 execCommand", async () => {
    setClipboard(undefined);
    const exec = vi.fn().mockReturnValue(true);
    (document as unknown as { execCommand: unknown }).execCommand = exec;

    await copyText("hello");

    expect(exec).toHaveBeenCalled();
  });

  it("所有方式都失败时抛出错误", async () => {
    setClipboard(undefined);
    (document as unknown as { execCommand: unknown }).execCommand = vi.fn().mockReturnValue(false);

    await expect(copyText("hello")).rejects.toThrow("copy failed");
  });
});
