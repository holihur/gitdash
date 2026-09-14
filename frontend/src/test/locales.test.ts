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

const LOCALES: Record<string, Record<string, unknown>> = {
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
  it("每个语言的 key 都与 en 完全对齐", () => {
    const enKeys = leafPaths(en).sort();
    for (const [name, messages] of Object.entries(LOCALES)) {
      expect(leafPaths(messages).sort(), `${name} keys`).toEqual(enKeys);
    }
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
    for (const [name, messages] of Object.entries(LOCALES)) check(messages, name);
  });

  it("占位符集合与 en 保持一致", () => {
    const placeholders = (s: string) => (s.match(/\{\w+\}/g) ?? []).sort().join(",");
    const enFlat: Record<string, string> = {};
    const flat = (obj: Record<string, unknown>, prefix = "") => {
      for (const [k, v] of Object.entries(obj)) {
        const p = prefix ? `${prefix}.${k}` : k;
        if (v && typeof v === "object") flat(v as Record<string, unknown>, p);
        else enFlat[p] = v as string;
      }
    };
    flat(en);
    for (const [name, messages] of Object.entries(LOCALES)) {
      const flatLocale: Record<string, string> = {};
      const walk = (obj: Record<string, unknown>, prefix = "") => {
        for (const [k, v] of Object.entries(obj)) {
          const p = prefix ? `${prefix}.${k}` : k;
          if (v && typeof v === "object") walk(v as Record<string, unknown>, p);
          else flatLocale[p] = v as string;
        }
      };
      walk(messages);
      for (const key of Object.keys(enFlat)) {
        expect(placeholders(flatLocale[key]), `${name}:${key} placeholders`).toBe(
          placeholders(enFlat[key]),
        );
      }
    }
  });
});
