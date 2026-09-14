import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { en, type Messages } from "@/locales/en";
import { zhCN } from "@/locales/zh-CN";
import { ja } from "@/locales/ja";
import { ko } from "@/locales/ko";
import { fr } from "@/locales/fr";
import { de } from "@/locales/de";
import { ru } from "@/locales/ru";
import { es } from "@/locales/es";
import { pt } from "@/locales/pt";

export type Lang =
  | "en"
  | "zh-CN"
  | "ja"
  | "ko"
  | "fr"
  | "de"
  | "ru"
  | "es"
  | "pt";

export const LANGS: { value: Lang; label: string }[] = [
  { value: "zh-CN", label: "中文" },
  { value: "en", label: "English" },
  { value: "ja", label: "日本語" },
  { value: "ko", label: "한국어" },
  { value: "fr", label: "Français" },
  { value: "de", label: "Deutsch" },
  { value: "ru", label: "Русский" },
  { value: "es", label: "Español" },
  { value: "pt", label: "Português" },
];

/** 各语言的 BCP-47 区域标记，用于 Intl 日期/数字格式化 */
export const DATE_LOCALES: Record<Lang, string> = {
  en: "en-US",
  "zh-CN": "zh-CN",
  ja: "ja-JP",
  ko: "ko-KR",
  fr: "fr-FR",
  de: "de-DE",
  ru: "ru-RU",
  es: "es-ES",
  pt: "pt-BR",
};

/** 取当前语言对应的 Intl 区域标记 */
export function dateLocale(lang: Lang): string {
  return DATE_LOCALES[lang] ?? "en-US";
}

const MESSAGES: Record<Lang, Messages> = {
  en,
  "zh-CN": zhCN,
  ja,
  ko,
  fr,
  de,
  ru,
  es,
  pt,
};

const STORAGE_KEY = "gitdash-lang";

// 浏览器语言前缀 → 支持的语言（zh 特判为 zh-CN）
const BROWSER_LANG: Record<string, Lang> = {
  zh: "zh-CN",
  en: "en",
  ja: "ja",
  ko: "ko",
  fr: "fr",
  de: "de",
  ru: "ru",
  es: "es",
  pt: "pt",
};

function detectLang(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved && saved in MESSAGES) return saved as Lang;
  } catch {
    /* ignore */
  }
  const nav = typeof navigator !== "undefined" ? navigator.language?.toLowerCase() : "";
  if (nav) {
    const base = nav.split("-")[0];
    if (nav.startsWith("zh")) return "zh-CN";
    if (base in BROWSER_LANG) return BROWSER_LANG[base];
  }
  return "en";
}

function resolve(obj: unknown, path: string): unknown {
  return path.split(".").reduce<unknown>((acc, key) => {
    if (acc && typeof acc === "object" && key in (acc as Record<string, unknown>)) {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, obj);
}

interface I18nContextValue {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
  /** 同 t，但 key 不存在时返回 undefined（用于可选翻译，如错误码） */
  to: (key: string, vars?: Record<string, string | number>) => string | undefined;
}

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectLang);

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, lang);
    } catch {
      /* ignore */
    }
    document.documentElement.lang = lang;
  }, [lang]);

  const setLang = useCallback((l: Lang) => setLangState(l), []);

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const template = (resolve(MESSAGES[lang], key) ??
        resolve(MESSAGES.en, key) ??
        key) as unknown;
      if (typeof template !== "string") return key;
      if (!vars) return template;
      return template.replace(/\{(\w+)\}/g, (_, name: string) => String(vars[name] ?? `{${name}}`));
    },
    [lang],
  );

  const to = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const template = (resolve(MESSAGES[lang], key) ?? resolve(MESSAGES.en, key)) as unknown;
      if (typeof template !== "string") return undefined;
      if (!vars) return template;
      return template.replace(/\{(\w+)\}/g, (_, name: string) => String(vars[name] ?? `{${name}}`));
    },
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t, to }), [lang, setLang, t, to]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}
