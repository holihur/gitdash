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

// i18n 只维护 zh-CN（完整）与 en；其余语言可只提供部分翻译，缺失 key 运行时回退英文。
const COMPLETE_LOCALES: Record<string, Record<string, unknown>> = { "zh-CN": zhCN };
const PARTIAL_LOCALES: Record<string, Record<string, unknown>> = { ja, ko, fr, de, ru, es, pt };
const ALL_LOCALES = { ...COMPLETE_LOCALES, ...PARTIAL_LOCALES };

describe("locales", () => {
  it("中文与英文 key 完全对齐", () => {
    expect(leafPaths(zhCN).sort()).toEqual(leafPaths(en).sort());
  });

  it("其余语言只允许 en 的子集（缺失 key 运行时回退英文）", () => {
    const enKeys = new Set(leafPaths(en));
    for (const [name, messages] of Object.entries(PARTIAL_LOCALES)) {
      for (const key of leafPaths(messages)) {
        expect(enKeys.has(key), `${name} 含未知 key ${key}`).toBe(true);
      }
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

  it("已翻译 key 的占位符与 en 保持一致", () => {
    const enFlat = flatten(en);
    for (const [name, messages] of Object.entries({ "zh-CN": zhCN, ...PARTIAL_LOCALES })) {
      for (const [key, value] of Object.entries(flatten(messages))) {
        expect(placeholders(value), `${name}:${key} placeholders`).toBe(
          placeholders(enFlat[key] ?? ""),
        );
      }
    }
  });
});
