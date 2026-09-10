import { useState } from "react";

import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { Textarea } from "@/components/ui/textarea";
import { MarkdownView } from "@/components/markdown";

/** Markdown 输入框：Write / Preview 切换，用于 issue/PR 描述与评论。 */
export function MarkdownEditor({
  id,
  value,
  onChange,
  placeholder,
  rows = 4,
  className,
}: {
  id?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  rows?: number;
  className?: string;
}) {
  const { t } = useI18n();
  const [mode, setMode] = useState<"write" | "preview">("write");

  return (
    <div className={cn("space-y-1.5", className)}>
      <div className="flex items-center gap-1 text-xs">
        {(["write", "preview"] as const).map((m) => (
          <button
            key={m}
            type="button"
            onClick={() => setMode(m)}
            className={cn(
              "rounded px-2 py-0.5 font-medium",
              mode === m
                ? "bg-muted text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {m === "write" ? t("common.write") : t("common.preview")}
          </button>
        ))}
        <span className="ml-auto text-[11px] text-muted-foreground">
          {t("common.markdownSupported")}
        </span>
      </div>
      {mode === "write" ? (
        <Textarea
          id={id}
          rows={rows}
          placeholder={placeholder}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <div className="min-h-24 rounded-md border bg-background px-3 py-2">
          {value.trim() ? (
            <MarkdownView text={value} />
          ) : (
            <p className="text-sm text-muted-foreground">{t("common.nothingToPreview")}</p>
          )}
        </div>
      )}
    </div>
  );
}
