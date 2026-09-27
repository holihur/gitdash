import { describe, expect, it } from "vitest";
import { en } from "@/locales/en";
import { zhCN } from "@/locales/zh-CN";
import { ja } from "@/locales/ja";
import { ko } from "@/locales/ko";
import { fr } from "@/locales/fr";
import { de } from "@/locales/de";
import { ru } from "@/locales/ru";
import { es } from "@/locales/es";
import { pt } from "@/locales/pt";

/** 递归收集对象的叶子 key 路径 */
function leafPaths(obj: Record<string, unknown>, prefix = ""): string[] {
  return Object.entries(obj).flatMap(([k, v]) => {
    const p = prefix ? `${prefix}.${k}` : k;
    return v && typeof v === "object" ? leafPaths(v as Record<string, unknown>, p) : [p];
  });
}

/** 展平为 key -> 字符串 */
function flatten(obj: Record<string, unknown>, prefix = ""): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(obj)) {
    const p = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object") Object.assign(out, flatten(v as Record<string, unknown>, p));
    else out[p] = v as string;
  }
  return out;
}

const placeholders = (s: string) => (s.match(/\{\w+\}/g) ?? []).sort().join(",");

// 所有语言都必须与 en 的 key 完全对齐：类型上已由 Messages 约束，这里再兜一层运行时校验，
// 避免有人把某个语言改回 DeepPartial 后静默回退英文。
const ALL_LOCALES: Record<string, Record<string, unknown>> = {
  "zh-CN": zhCN,
  ja,
  ko,
  fr,
  de,
  ru,
  es,
  pt,
};

describe("locales", () => {
  it("所有语言与英文 key 完全对齐", () => {
    const enKeys = leafPaths(en).sort();
    for (const [name, messages] of Object.entries(ALL_LOCALES)) {
      expect(leafPaths(messages).sort(), `${name} 的 key 集合与 en 不一致`).toEqual(enKeys);
    }
  });

  it("叶子值均为非空字符串", () => {
    for (const [name, messages] of Object.entries({ en, ...ALL_LOCALES })) {
      const flat = flatten(messages);
      for (const [key, value] of Object.entries(flat)) {
        const bad = typeof value !== "string" || value.length === 0;
        if (bad) throw new Error(`${name}:${key} = ${JSON.stringify(value)}`);
      }
    }
  });

  it("占位符与 en 保持一致", () => {
    const enFlat = flatten(en);
    for (const [name, messages] of Object.entries(ALL_LOCALES)) {
      for (const [key, value] of Object.entries(flatten(messages))) {
        expect(placeholders(value), `${name}:${key} placeholders`).toBe(
          placeholders(enFlat[key] ?? ""),
        );
      }
    }
  });
});
