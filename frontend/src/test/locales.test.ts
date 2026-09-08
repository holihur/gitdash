import { describe, expect, it } from "vitest";
import { en } from "@/locales/en";
import { zhCN } from "@/locales/zh-CN";

/** 递归收集对象的叶子 key 路径 */
function leafPaths(obj: Record<string, unknown>, prefix = ""): string[] {
  return Object.entries(obj).flatMap(([k, v]) => {
    const p = prefix ? `${prefix}.${k}` : k;
    return v && typeof v === "object" ? leafPaths(v as Record<string, unknown>, p) : [p];
  });
}

describe("locales", () => {
  it("en 与 zh-CN 的 key 完全对齐", () => {
    const enKeys = leafPaths(en).sort();
    const zhKeys = leafPaths(zhCN).sort();
    expect(zhKeys).toEqual(enKeys);
  });

  it("叶子值均为非空字符串", () => {
    const check = (obj: Record<string, unknown>, name: string) => {
      const walk = (o: Record<string, unknown>, prefix: string) => {
        for (const [k, v] of Object.entries(o)) {
          if (v && typeof v === "object") walk(v as Record<string, unknown>, `${prefix}.${k}`);
          else {
            const bad = typeof v !== "string" || (v as string).length === 0;
            if (bad) throw new Error(`${name}:${prefix}.${k} = ${JSON.stringify(v)}`);
          }
        }
      };
      walk(obj, "");
    };
    check(en, "en");
    check(zhCN, "zh-CN");
  });
});
