import { describe, expect, it } from "vitest";
import { formatDate, formatRelativeTime } from "@/lib/utils";

const NOW = Date.parse("2025-06-15T12:00:00Z");
const ago = (ms: number) => new Date(NOW - ms).toISOString();

describe("formatRelativeTime", () => {
  it("45 秒内显示“现在”", () => {
    expect(formatRelativeTime(ago(10_000), "en-US", NOW)).toBe("now");
    expect(formatRelativeTime(ago(0), "zh-CN", NOW)).toBe("现在");
  });

  it("分钟 / 小时级相对时间", () => {
    expect(formatRelativeTime(ago(5 * 60_000), "en-US", NOW)).toBe("5 minutes ago");
    expect(formatRelativeTime(ago(3 * 3_600_000), "zh-CN", NOW)).toBe("3小时前");
  });

  it("昨天用自然语言表达", () => {
    expect(formatRelativeTime(ago(86_400_000), "en-US", NOW)).toBe("yesterday");
    expect(formatRelativeTime(ago(86_400_000), "zh-CN", NOW)).toBe("昨天");
  });

  it("未来时间同样人性化", () => {
    expect(formatRelativeTime(new Date(NOW + 2 * 3_600_000).toISOString(), "en-US", NOW)).toBe(
      "in 2 hours",
    );
  });

  it("超过 30 天回退到绝对日期", () => {
    const iso = ago(40 * 86_400_000);
    expect(formatRelativeTime(iso, "en-US", NOW)).toBe(formatDate(iso, "en-US"));
  });

  it("非法时间原样返回", () => {
    expect(formatRelativeTime("not-a-date", "en-US", NOW)).toBe("not-a-date");
  });
});
