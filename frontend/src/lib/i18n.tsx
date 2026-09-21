import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { en, type DeepPartial, type Messages } from "@/locales/en";

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

type Msg = Messages | DeepPartial<Messages>;

// 语言包按需加载：只有 en 静态打包（作为缺失 key 的兜底），其余语言在切换/启动时
// 才下载对应 chunk。此前 9 种语言全部静态打包进首屏公共 chunk（约 500KB 源码），
// 是首屏体积的最大来源。
const LOADERS: Record<Lang, () => Promise<Msg>> = {
  en: async () => en,
  "zh-CN": () => import("@/locales/zh-CN").then((m) => m.zhCN),
  ja: () => import("@/locales/ja").then((m) => m.ja),
  ko: () => import("@/locales/ko").then((m) => m.ko),
  fr: () => import("@/locales/fr").then((m) => m.fr),
  de: () => import("@/locales/de").then((m) => m.de),
  ru: () => import("@/locales/ru").then((m) => m.ru),
  es: () => import("@/locales/es").then((m) => m.es),
  pt: () => import("@/locales/pt").then((m) => m.pt),
};

// 已加载语言包的模块级缓存：跨 Provider 实例/重挂载复用，且首次渲染可同步读取。
const cache: Partial<Record<Lang, Msg>> = { en };

/**
 * 预加载指定语言（幂等）。应用启动时先 await 当前语言，避免首帧出现英文兜底文案；
 * 运行时切换语言也走这里，加载完成后再触发重渲染。
 */
export async function preloadLang(lang: Lang): Promise<void> {
  if (cache[lang]) return;
  try {
    cache[lang] = await LOADERS[lang]();
  } catch {
    /* 加载失败时保持英文兜底 */
  }
}

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

export function detectLang(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved && saved in LOADERS) return saved as Lang;
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
  // 语言包集合快照：动态 import 完成后替换引用，驱动 t/to 重新计算。
  const [store, setStore] = useState<Partial<Record<Lang, Msg>>>(() => ({ ...cache }));

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, lang);
    } catch {
      /* ignore */
    }
    document.documentElement.lang = lang;
    // 直接挂载 Provider（如管理台 / 测试）时懒加载当前语言。
    if (!cache[lang]) void preloadLang(lang).then(() => setStore({ ...cache }));
  }, [lang]);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    if (!cache[l]) void preloadLang(l).then(() => setStore({ ...cache }));
  }, []);

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const template = (resolve(store[lang], key) ?? resolve(en, key) ?? key) as unknown;
      if (typeof template !== "string") return key;
      if (!vars) return template;
      return template.replace(/\{(\w+)\}/g, (_, name: string) => String(vars[name] ?? `{${name}}`));
    },
    [lang, store],
  );

  const to = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      const template = (resolve(store[lang], key) ?? resolve(en, key)) as unknown;
      if (typeof template !== "string") return undefined;
      if (!vars) return template;
      return template.replace(/\{(\w+)\}/g, (_, name: string) => String(vars[name] ?? `{${name}}`));
    },
    [lang, store],
  );

  const value = useMemo(() => ({ lang, setLang, t, to }), [lang, setLang, t, to]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}
