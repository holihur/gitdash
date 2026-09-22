import { useEffect, useMemo, useState } from "react";
import { Palette, RotateCcw } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { languageColor } from "@/lib/language-color";
import { adminReq } from "./api";

const HEX = /^#[0-9a-fA-F]{6}$/;

/**
 * 语言配色：为检测器支持的语言覆盖默认颜色。
 * 只保存“覆盖项”；把某项改回默认（重置）即移除覆盖。
 */
export function LanguageSettings() {
  const { t, to } = useI18n();
  const [languages, setLanguages] = useState<string[]>([]);
  const [defaults, setDefaults] = useState<Record<string, string>>({});
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const [filter, setFilter] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    let alive = true;
    setLoading(true);
    fetch("/api/languages")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error("load failed"))))
      .then((d: { languages?: string[]; defaults?: Record<string, string>; colors?: Record<string, string> }) => {
        if (!alive) return;
        setLanguages(Array.isArray(d.languages) ? d.languages : []);
        setDefaults(d.defaults && typeof d.defaults === "object" ? d.defaults : {});
        setOverrides(d.colors && typeof d.colors === "object" ? d.colors : {});
      })
      .catch((e) => {
        if (alive) setMsg(apiErrorMsg(to, e));
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [to]);

  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return languages.filter((l) => !q || l.toLowerCase().includes(q));
  }, [languages, filter]);

  // 覆盖值必须为合法 #RRGGBB，或空串（等待输入）
  const invalid = Object.entries(overrides).filter(([, v]) => v !== "" && !HEX.test(v));

  const setColor = (lang: string, value: string) =>
    setOverrides((prev) => ({ ...prev, [lang]: value }));
  const resetColor = (lang: string) =>
    setOverrides((prev) => {
      const next = { ...prev };
      delete next[lang];
      return next;
    });

  const save = async () => {
    if (invalid.length > 0) {
      setMsg(t("admin.languageColorsInvalid"));
      return;
    }
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/language-colors", { colors: overrides });
      setMsg(t("admin.saved"));
    } catch (e) {
      setMsg(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Palette className="h-4 w-4" />
          {t("admin.languageColorsTitle")}
        </CardTitle>
        <CardDescription>{t("admin.languageColorsHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="h-8 max-w-64"
            placeholder={t("admin.languageColorsFilter")}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={() => setOverrides({})}
            disabled={busy || Object.keys(overrides).length === 0}
          >
            <RotateCcw className="h-3.5 w-3.5" />
            {t("admin.languageColorsResetAll")}
          </Button>
        </div>

        <div className="max-h-96 overflow-auto rounded-md border">
          {loading ? (
            <p className="p-3 text-sm text-muted-foreground">{t("common.loading")}</p>
          ) : shown.length === 0 ? (
            <p className="p-3 text-sm text-muted-foreground">{t("admin.languageColorsEmpty")}</p>
          ) : (
            <ul className="grid gap-x-4 gap-y-1 p-2 sm:grid-cols-2">
              {shown.map((lang) => {
                const value = overrides[lang];
                const valid = value && HEX.test(value) ? value : "";
                const fallback = defaults[lang] || languageColor(lang);
                const shownColor = valid || fallback;
                const dirty = value !== undefined;
                return (
                  <li key={lang} className="flex items-center gap-2 py-1 text-sm">
                    <input
                      type="color"
                      aria-label={`${lang} color`}
                      value={shownColor}
                      onChange={(e) => setColor(lang, e.target.value)}
                      className="h-6 w-6 shrink-0 cursor-pointer rounded border bg-transparent p-0"
                    />
                    <span className="min-w-0 flex-1 truncate" title={lang}>
                      {lang}
                    </span>
                    <Input
                      className="h-7 w-24 shrink-0 font-mono text-xs"
                      value={value ?? ""}
                      placeholder={fallback}
                      onChange={(e) => setColor(lang, e.target.value)}
                      aria-label={`${lang} hex`}
                    />
                    <button
                      type="button"
                      onClick={() => resetColor(lang)}
                      disabled={!dirty}
                      title={t("admin.languageColorsReset")}
                      aria-label={`${t("admin.languageColorsReset")} ${lang}`}
                      className="shrink-0 text-muted-foreground disabled:opacity-30"
                    >
                      <RotateCcw className="h-3.5 w-3.5" />
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>

        {invalid.length > 0 && (
          <p className="text-sm text-destructive">{t("admin.languageColorsInvalid")}</p>
        )}
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy || invalid.length > 0}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
