import { useI18n } from "@/lib/i18n";
import { useLanguageColor } from "@/lib/language-colors";
import type { LanguageStat } from "@/lib/api";

/** 百分比展示：保留一位小数，极小值显示 <0.1%。 */
export function formatPercent(p: number): string {
  if (p > 0 && p < 0.1) return "<0.1";
  return p.toFixed(1);
}

/** 主要语言徽标：彩色圆点 + 语言名，用于仓库列表卡片。 */
export function LanguageBadge({ language, className }: { language?: string; className?: string }) {
  const colorFor = useLanguageColor();
  if (!language) return null;
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs ${className ?? ""}`}>
      <span
        className="h-2.5 w-2.5 shrink-0 rounded-full"
        style={{ backgroundColor: colorFor(language) }}
      />
      <span className="font-medium">{language}</span>
    </span>
  );
}

/**
 * 语言构成条：堆叠色条 + top5 图例。languages 已按占比降序，
 * 若 top5 之外仍有内容，补一段 “Other”。
 */
export function LanguageBar({
  languages,
  className,
}: {
  languages: LanguageStat[];
  className?: string;
}) {
  const { t } = useI18n();
  const colorFor = useLanguageColor();
  if (!languages || languages.length === 0) return null;
  const sum = languages.reduce((acc, l) => acc + l.percent, 0);
  const other = Math.max(0, 100 - sum);

  return (
    <div className={className}>
      <div className="flex h-2 w-full overflow-hidden rounded-full bg-muted">
        {languages.map((l) => (
          <span
            key={l.language}
            style={{ width: `${l.percent}%`, backgroundColor: colorFor(l.language) }}
            title={`${l.language} ${formatPercent(l.percent)}%`}
          />
        ))}
        {other > 0.05 && (
          <span
            style={{ width: `${other}%`, backgroundColor: "rgb(148 163 184)" }}
            title={`${t("repo.languageOther")} ${formatPercent(other)}%`}
          />
        )}
      </div>
      <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {languages.map((l) => (
          <li key={l.language} className="flex items-center gap-1.5">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: colorFor(l.language) }}
            />
            <span className="font-medium text-foreground">{l.language}</span>
            <span>{formatPercent(l.percent)}%</span>
          </li>
        ))}
        {other > 0.05 && (
          <li className="flex items-center gap-1.5">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: "rgb(148 163 184)" }}
            />
            <span className="font-medium text-foreground">{t("repo.languageOther")}</span>
            <span>{formatPercent(other)}%</span>
          </li>
        )}
      </ul>
    </div>
  );
}
