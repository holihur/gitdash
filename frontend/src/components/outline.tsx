import { useMemo } from "react";
import { extractOutline, type OutlineItem } from "@/lib/outline";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

interface OutlineProps {
  path: string;
  content: string;
  /** 跳转到锚点（Markdown 标题） */
  onJumpToId?: (id: string) => void;
  /** 跳转到行号（代码符号） */
  onJumpToLine?: (line: number) => void;
}

const KIND_LABEL: Record<OutlineItem["kind"], string> = {
  heading: "",
  function: "ƒ",
  type: "▦",
  variable: "=",
  section: "§",
};

/**
 * 代码浏览右侧大纲：Markdown 标题 / 代码符号列表，点击跳转。
 */
export default function Outline({ path, content, onJumpToId, onJumpToLine }: OutlineProps) {
  const { t } = useI18n();
  const items = useMemo(() => extractOutline(path, content), [path, content]);

  if (items.length === 0) {
    return (
      <p className="px-2 py-3 text-xs text-muted-foreground">{t("outline.empty")}</p>
    );
  }

  return (
    <div className="space-y-0.5">
      {items.map((item, i) => {
        const jump = () => {
          if (item.id) onJumpToId?.(item.id);
          else if (item.line) onJumpToLine?.(item.line);
        };
        return (
          <button
            key={`${item.id ?? item.line ?? ""}-${i}`}
            type="button"
            onClick={jump}
            title={`${item.line ? `L${item.line} · ` : ""}${item.text}`}
            className={cn(
              "flex w-full items-center gap-1.5 truncate rounded px-1.5 py-0.5 text-left font-mono text-xs text-muted-foreground hover:bg-muted hover:text-foreground",
              item.level === 1 && "pl-4",
              item.level >= 2 && "pl-7",
            )}
          >
            {KIND_LABEL[item.kind] && (
              <span className="w-3 shrink-0 text-[10px] text-muted-foreground/70">
                {KIND_LABEL[item.kind]}
              </span>
            )}
            <span className="truncate">{item.text}</span>
          </button>
        );
      })}
    </div>
  );
}
