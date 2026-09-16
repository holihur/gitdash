import { useCallback, useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { ChevronDown, ChevronUp, FileText, Search, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useI18n } from "@/lib/i18n";
import {
  groupLines,
  parsePatch,
  statusTextClass,
  type DiffFileInfo,
  type DiffLineContext,
} from "@/components/diff-parse";

/** 大小写不敏感地高亮命中的子串 */
function HighlightedText({
  text,
  query,
  current,
}: {
  text: string;
  query: string;
  current: boolean;
}) {
  if (!query.trim() || !text) return <>{text || " "}</>;
  const lower = text.toLowerCase();
  const needle = query.toLowerCase();
  const nodes: ReactNode[] = [];
  let cursor = 0;
  let key = 0;
  while (cursor < text.length) {
    const hit = lower.indexOf(needle, cursor);
    if (hit === -1) {
      nodes.push(text.slice(cursor));
      break;
    }
    if (hit > cursor) nodes.push(text.slice(cursor, hit));
    nodes.push(
      <mark
        key={key++}
        className={
          current
            ? "rounded-sm bg-amber-400/70 text-inherit"
            : "rounded-sm bg-amber-300/40 text-inherit"
        }
      >
        {text.slice(hit, hit + needle.length)}
      </mark>,
    );
    cursor = hit + needle.length;
  }
  return <>{nodes}</>;
}

export interface SplitDiffProps {
  files: DiffFileInfo[];
  patch: string;
  /** 右侧行内操作（如行评论按钮） */
  renderLineRight?: (ctx: DiffLineContext) => ReactNode;
  /** 行下方附加内容（如评论列表与输入框） */
  renderLineFooter?: (ctx: DiffLineContext) => ReactNode;
  heightClass?: string;
}

interface DiffMatch {
  file: string;
  chunk: number;
  line: number;
}

/** 左右分栏的 diff 浏览器：左侧文件列表 + 搜索，右侧单文件 diff + 搜索高亮 */
export default function SplitDiff({
  files,
  patch,
  renderLineRight,
  renderLineFooter,
  heightClass = "max-h-[60vh]",
}: SplitDiffProps) {
  const { t } = useI18n();
  const uid = useId();
  const scrollRef = useRef<HTMLDivElement>(null);

  const parsed = useMemo(() => parsePatch(patch), [patch]);
  const chunks = useMemo(() => groupLines(parsed), [parsed]);

  const fileList = useMemo(() => {
    const seen = new Set<string>();
    const list: DiffFileInfo[] = [];
    for (const f of files) {
      if (seen.has(f.path)) continue;
      seen.add(f.path);
      list.push(f);
    }
    for (const c of chunks) {
      if (c.path && !seen.has(c.path)) {
        seen.add(c.path);
        list.push({ path: c.path, status: "M", insertions: 0, deletions: 0 });
      }
    }
    return list;
  }, [files, chunks]);

  // 无 diff 头（例如纯 patch）时，把唯一分块归到首个文件，保证能被选中。
  const normalizedChunks = useMemo(() => {
    if (chunks.length === 1 && chunks[0].path === "" && fileList.length > 0) {
      return [{ ...chunks[0], path: fileList[0].path }];
    }
    return chunks;
  }, [chunks, fileList]);

  const [selected, setSelected] = useState("");
  const [fileQuery, setFileQuery] = useState("");
  const [query, setQuery] = useState("");
  const [matchIndex, setMatchIndex] = useState(0);

  useEffect(() => {
    if (fileList.length === 0) {
      if (selected) setSelected("");
      return;
    }
    if (!selected || !fileList.some((f) => f.path === selected)) {
      setSelected(fileList[0].path);
    }
  }, [fileList, selected]);

  const filteredFiles = useMemo(() => {
    const q = fileQuery.trim().toLowerCase();
    if (!q) return fileList;
    return fileList.filter((f) => f.path.toLowerCase().includes(q));
  }, [fileList, fileQuery]);

  const selectedChunkIndex = useMemo(
    () => normalizedChunks.findIndex((c) => c.path === selected),
    [normalizedChunks, selected],
  );
  const selectedChunk = selectedChunkIndex >= 0 ? normalizedChunks[selectedChunkIndex] : null;
  const selectedInfo = useMemo(
    () => fileList.find((f) => f.path === selected),
    [fileList, selected],
  );

  const matches = useMemo<DiffMatch[]>(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return [];
    const out: DiffMatch[] = [];
    normalizedChunks.forEach((chunk, chunkIndex) => {
      chunk.lines.forEach((line, lineIndex) => {
        if (line.raw.toLowerCase().includes(needle)) {
          out.push({ file: chunk.path, chunk: chunkIndex, line: lineIndex });
        }
      });
    });
    return out;
  }, [normalizedChunks, query]);

  useEffect(() => {
    setMatchIndex(0);
  }, [query]);

  const currentMatch = matches.length ? matches[Math.min(matchIndex, matches.length - 1)] : null;

  useEffect(() => {
    if (!currentMatch) return;
    const el = document.getElementById(`${uid}-${currentMatch.chunk}-${currentMatch.line}`);
    el?.scrollIntoView?.({ block: "center" });
  }, [currentMatch, uid]);

  const gotoMatch = useCallback(
    (delta: number) => {
      if (matches.length === 0) return;
      const next = (matchIndex + delta + matches.length) % matches.length;
      setMatchIndex(next);
      setSelected(matches[next].file);
    },
    [matchIndex, matches],
  );

  if (!patch) {
    return <p className="text-xs text-muted-foreground">-</p>;
  }

  return (
    <div className="flex flex-col gap-2 md:flex-row">
      <aside className="flex w-full shrink-0 flex-col gap-2 md:w-64">
        <div className="relative">
          <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={fileQuery}
            onChange={(e) => setFileQuery(e.target.value)}
            placeholder={t("diff.filterFiles")}
            aria-label={t("diff.filterFiles")}
            className="h-8 pl-7 text-xs"
          />
        </div>
        <div className="max-h-40 overflow-auto rounded-md border md:max-h-[60vh]">
          {filteredFiles.length === 0 ? (
            <p className="px-2 py-3 text-center text-xs text-muted-foreground">
              {t("diff.noFiles")}
            </p>
          ) : (
            filteredFiles.map((f) => {
              const active = f.path === selected;
              return (
                <button
                  key={f.path}
                  type="button"
                  onClick={() => setSelected(f.path)}
                  title={`${f.path} (+${f.insertions} -${f.deletions})`}
                  className={`flex w-full items-center gap-1.5 border-b border-border/40 px-2 py-1.5 text-left font-mono text-xs last:border-b-0 ${
                    active ? "bg-accent text-accent-foreground" : "hover:bg-muted/60"
                  }`}
                >
                  <span className={`shrink-0 ${statusTextClass(f.status)}`}>{f.status}</span>
                  <span className="min-w-0 flex-1 truncate">{f.path}</span>
                  <span className="shrink-0 text-[10px] text-muted-foreground">
                    +{f.insertions} -{f.deletions}
                  </span>
                </button>
              );
            })
          )}
        </div>
      </aside>

      <section className="min-w-0 flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex min-w-0 flex-1 items-center gap-2 font-mono text-xs">
            <FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate" title={selected || undefined}>
              {selected || "-"}
            </span>
            {selectedInfo && (
              <Badge variant="outline" className="shrink-0 gap-1 font-mono text-[10px]">
                <span className={statusTextClass(selectedInfo.status)}>{selectedInfo.status}</span>
                <span className="text-muted-foreground">
                  +{selectedInfo.insertions} -{selectedInfo.deletions}
                </span>
              </Badge>
            )}
          </div>
          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("diff.searchPlaceholder")}
              aria-label={t("diff.searchPlaceholder")}
              className="h-8 pl-7 pr-20 text-xs"
            />
            {query && (
              <div className="absolute right-1 top-1/2 flex -translate-y-1/2 items-center gap-0.5">
                <span className="px-1 text-[10px] text-muted-foreground">
                  {t("diff.matchCount", {
                    current: matches.length ? Math.min(matchIndex + 1, matches.length) : 0,
                    total: matches.length,
                  })}
                </span>
                <button
                  type="button"
                  onClick={() => gotoMatch(-1)}
                  disabled={!matches.length}
                  title={t("diff.prevMatch")}
                  aria-label={t("diff.prevMatch")}
                  className="rounded p-0.5 text-muted-foreground hover:text-foreground disabled:opacity-40"
                >
                  <ChevronUp className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => gotoMatch(1)}
                  disabled={!matches.length}
                  title={t("diff.nextMatch")}
                  aria-label={t("diff.nextMatch")}
                  className="rounded p-0.5 text-muted-foreground hover:text-foreground disabled:opacity-40"
                >
                  <ChevronDown className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => setQuery("")}
                  title={t("diff.clearSearch")}
                  aria-label={t("diff.clearSearch")}
                  className="rounded p-0.5 text-muted-foreground hover:text-foreground"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
            )}
          </div>
        </div>

        {selectedChunk ? (
          <div
            ref={scrollRef}
            className={`overflow-auto rounded-md border bg-background/60 ${heightClass}`}
          >
            <div className="min-w-max py-2 font-mono text-xs leading-5">
              {selectedChunk.lines.map((pl, i) => {
                const id = `${uid}-${selectedChunkIndex}-${i}`;
                const ctx: DiffLineContext = {
                  line: pl,
                  file: selectedChunk.path,
                  chunk: selectedChunkIndex,
                  index: i,
                  id,
                };
                const isCurrent =
                  !!currentMatch && currentMatch.chunk === selectedChunkIndex && currentMatch.line === i;
                return (
                  <div key={i} id={id}>
                    <div
                      className={`group relative flex ${pl.cls}`}
                      title={
                        selectedChunk.path && (pl.new != null || pl.old != null)
                          ? `${selectedChunk.path}:${pl.new ?? pl.old}`
                          : undefined
                      }
                    >
                      <span className="sticky left-0 w-10 shrink-0 select-none border-r border-border/40 bg-background/80 px-1 text-right text-[10px] text-muted-foreground">
                        {pl.old ?? ""}
                      </span>
                      <span className="w-10 shrink-0 select-none border-r border-border/40 bg-background/80 px-1 text-right text-[10px] text-muted-foreground">
                        {pl.new ?? ""}
                      </span>
                      <span className="flex-1 whitespace-pre px-2">
                        <HighlightedText text={pl.raw} query={query} current={isCurrent} />
                      </span>
                      {renderLineRight?.(ctx)}
                    </div>
                    {renderLineFooter?.(ctx)}
                  </div>
                );
              })}
            </div>
          </div>
        ) : (
          <p className="rounded-md border px-3 py-6 text-center text-xs text-muted-foreground">
            {t("diff.noFiles")}
          </p>
        )}
      </section>
    </div>
  );
}
