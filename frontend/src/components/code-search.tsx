import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Search } from "lucide-react";
import { toast } from "sonner";
import { api, type SearchResult } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export function HighlightText({ text, q }: { text: string; q: string }) {
  if (!q) return <>{text}</>;
  const idx = text.toLowerCase().indexOf(q.toLowerCase());
  if (idx < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="rounded-sm bg-yellow-200/80 text-inherit dark:bg-yellow-500/30">
        {text.slice(idx, idx + q.length)}
      </mark>
      {text.slice(idx + q.length)}
    </>
  );
}

/** 代码搜索：防抖 300ms，Enter 立即搜索 */

export function CodeSearch({
  owner,
  name,
  refName,
  setParams,
  actions,
}: {
  owner: string;
  name: string;
  refName: string;
  setParams: (patch: Record<string, string | null>) => void;
  /** 与搜索框同行的操作按钮（新建文件/文件夹等） */
  actions?: ReactNode;
}) {
  const { t } = useI18n();
  const [q, setQ] = useState("");
  const [results, setResults] = useState<SearchResult[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [expanded, setExpanded] = useState(true);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const run = useMemo(
    () => async (query: string) => {
      const trimmed = query.trim();
      if (!trimmed) {
        setResults(null);
        return;
      }
      setLoading(true);
      try {
        setResults(await api.searchRepo(owner, name, trimmed, refName || undefined));
      } catch (e) {
        toast.error(e instanceof Error ? e.message : String(e));
        setResults([]);
      } finally {
        setLoading(false);
      }
    },
    [owner, name, refName],
  );

  const onChange = (value: string) => {
    setQ(value);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => run(value), 300);
  };

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-0 flex-1 basis-52">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={q}
            onChange={(e) => onChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                if (timer.current) clearTimeout(timer.current);
                run(q);
              }
            }}
            placeholder={t("search.placeholder")}
            className="h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        {actions}
      </div>
      {results !== null && (
        <div className="rounded-lg border bg-card">
          <button
            type="button"
            className="flex w-full items-center gap-2 border-b px-3 py-2 text-xs text-muted-foreground"
            onClick={() => setExpanded((v) => !v)}
          >
            {loading ? "…" : t("search.count", { count: results.length })}
            <span className="ml-auto">{expanded ? "▾" : "▸"}</span>
          </button>
          {expanded && (
            <div className="max-h-72 overflow-auto p-1">
              {results.length === 0 && !loading && (
                <p className="px-3 py-2 text-xs text-muted-foreground">{t("search.empty")}</p>
              )}
              {results.map((r, i) => (
                <button
                  key={`${r.path}:${r.line}:${i}`}
                  className="block w-full rounded px-2 py-1 text-left text-xs hover:bg-muted"
                  onClick={() => setParams({ file: r.path, line: String(r.line) })}
                  title={`${r.path}:${r.line}`}
                >
                  <span className="mr-2 font-mono font-medium">{r.path}:{r.line}</span>
                  <span className="font-mono text-muted-foreground">
                    <HighlightText text={r.text} q={q.trim()} />
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

