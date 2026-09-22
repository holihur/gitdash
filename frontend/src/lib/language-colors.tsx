import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { languageColor } from "./language-color";

export interface LanguageColors {
  /** 后端提供的默认配色（language -> #RRGGBB）。 */
  defaults: Record<string, string>;
  /** 管理端配置的覆盖配色（language -> #RRGGBB）。 */
  overrides: Record<string, string>;
}

const EMPTY: LanguageColors = { defaults: {}, overrides: {} };

const LanguageColorsContext = createContext<LanguageColors>(EMPTY);

/**
 * 加载语言配色（`GET /api/languages`：defaults + colors）并向下提供。
 * 后端不可用时保持空值，取色会回退到内置调色板，不影响展示。
 */
export function LanguageColorsProvider({ children }: { children: ReactNode }) {
  const [colors, setColors] = useState<LanguageColors>(EMPTY);
  useEffect(() => {
    let alive = true;
    fetch("/api/languages")
      .then((r) => (r.ok ? r.json() : null))
      .then((data: { defaults?: Record<string, string>; colors?: Record<string, string> } | null) => {
        if (!alive || !data) return;
        setColors({
          defaults: data.defaults && typeof data.defaults === "object" ? data.defaults : {},
          overrides: data.colors && typeof data.colors === "object" ? data.colors : {},
        });
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);
  return <LanguageColorsContext.Provider value={colors}>{children}</LanguageColorsContext.Provider>;
}

/**
 * 返回取色函数：管理端覆盖 > 后端默认 > 内置调色板兜底。
 */
export function useLanguageColor() {
  const { defaults, overrides } = useContext(LanguageColorsContext);
  return useCallback(
    (lang: string) => overrides[lang] || defaults[lang] || languageColor(lang),
    [defaults, overrides],
  );
}
