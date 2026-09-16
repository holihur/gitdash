import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { toast } from "sonner";
import { ChevronDown, ChevronUp, FileText, MessageSquare, Search, X } from "lucide-react";
import { api, type IssueComment } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { MarkdownView } from "@/components/markdown";
import { Textarea } from "@/components/ui/textarea";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export interface DiffFileInfo {
  path: string;
  status: "A" | "M" | "D";
  insertions: number;
  deletions: number;
}

interface ParsedLine {
  raw: string;
  cls: string;
  /** 命中行所属文件（diff --git 头解析） */
  file: string;
  old?: number;
  new?: number;
}

/** 解析 unified patch：为每行标注文件路径与 old/new 行号（文件头与 index 行无行号） */
function parsePatch(patch: string): ParsedLine[] {
  const out: ParsedLine[] = [];
  let file = "";
  let oldLine = 0;
  let newLine = 0;
  for (const raw of patch.split("\n")) {
    let cls = "";
    if (raw.startsWith("diff --git")) {
      const m = /^diff --git a\/(.*) b\/(.*)$/.exec(raw);
      file = m ? m[1] : "";
      cls = "text-muted-foreground";
    } else if (raw.startsWith("index ")) {
      cls = "text-muted-foreground";
    } else if (raw.startsWith("@@")) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(raw);
      if (m) {
        oldLine = Number(m[1]);
        newLine = Number(m[2]);
      }
      cls = "text-blue-600 dark:text-blue-400";
    } else if (raw.startsWith("+") && !raw.startsWith("+++")) {
      cls = "bg-green-500/15 text-green-700 dark:text-green-400";
      out.push({ raw, cls, file, new: newLine });
      newLine += 1;
      continue;
    } else if (raw.startsWith("-") && !raw.startsWith("---")) {
      cls = "bg-red-500/15 text-red-700 dark:text-red-400";
      out.push({ raw, cls, file, old: oldLine });
      oldLine += 1;
      continue;
    } else if (raw.startsWith("---") || raw.startsWith("+++")) {
      cls = "text-muted-foreground";
    } else if (raw !== "") {
      out.push({ raw, cls, file, old: oldLine, new: newLine });
      oldLine += 1;
      newLine += 1;
      continue;
    }
    out.push({ raw, cls, file });
  }
  return out;
}

interface FileChunk {
  path: string;
  lines: ParsedLine[];
}

/** 按文件对 patch 行分组；文件头之前的行并入首个文件 */
function groupLines(lines: ParsedLine[]): FileChunk[] {
  const chunks: FileChunk[] = [];
  const pending: ParsedLine[] = [];
  let current: FileChunk | null = null;

  for (const line of lines) {
    if (line.file) {
      if (!current || current.path !== line.file) {
        current = { path: line.file, lines: [] };
        if (pending.length) {
          current.lines.push(...pending);
          pending.length = 0;
        }
        chunks.push(current);
      }
      current.lines.push(line);
    } else if (current) {
      current.lines.push(line);
    } else {
      pending.push(line);
    }
  }

  if (pending.length) chunks.push({ path: "", lines: pending });
  return chunks;
}

const statusTextClass = (status: DiffFileInfo["status"]) =>
  status === "A"
    ? "text-green-600 dark:text-green-400"
    : status === "D"
      ? "text-red-600 dark:text-red-400"
      : "text-yellow-600 dark:text-yellow-400";

/** 行内评论定位键 */
const commentKey = (file: string, side: "old" | "new", line: number) => `${file}|${side}|${line}`;

interface CommentTarget {
  file: string;
  line: number;
  side: "old" | "new";
}

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

interface DiffLineContext {
  line: ParsedLine;
  file: string;
  chunk: number;
  index: number;
  id: string;
}

interface SplitDiffProps {
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
function SplitDiff({
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
                const isCurrent = !!currentMatch && currentMatch.chunk === selectedChunkIndex && currentMatch.line === i;
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

/** PR diff 展示：左侧文件列表、右侧单文件 patch、搜索与行级 hover 行内评论。 */
export function PullDiffView({
  owner,
  name,
  number,
  files,
  patch,
  canWrite,
}: {
  owner: string;
  name: string;
  number: number;
  files: DiffFileInfo[];
  patch: string;
  canWrite: boolean;
}) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [comments, setComments] = useState<IssueComment[]>([]);
  const [active, setActive] = useState<CommentTarget | null>(null);
  const [body, setBody] = useState("");
  const [posting, setPosting] = useState(false);

  const load = useCallback(async () => {
    try {
      const all = await api.listComments(owner, name, number, "pulls");
      setComments(all.filter((c) => c.file_path && c.line));
    } catch {
      /* ignore */
    }
  }, [owner, name, number]);

  useEffect(() => {
    load();
  }, [load]);

  const commentMap = useMemo(() => {
    const m = new Map<string, IssueComment[]>();
    for (const c of comments) {
      const side = c.line_side === "old" ? "old" : "new";
      const key = commentKey(c.file_path ?? "", side, c.line ?? 0);
      const arr = m.get(key) ?? [];
      arr.push(c);
      m.set(key, arr);
    }
    return m;
  }, [comments]);

  const targetOf = useCallback((file: string, line: ParsedLine): CommentTarget | null => {
    if (!file) return null;
    if (line.new == null && line.old == null) return null;
    return {
      file,
      line: (line.new ?? line.old) as number,
      side: line.new != null ? "new" : "old",
    };
  }, []);

  const post = async () => {
    if (!active || !body.trim()) return;
    setPosting(true);
    try {
      await api.postComment(owner, name, number, body.trim(), "pulls", {
        file_path: active.file,
        line: active.line,
        line_side: active.side,
      });
      toast.success(t("comments.posted"));
      setBody("");
      setActive(null);
      load();
    } catch (e) {
      toast.error(
        to("comments.failed", { error: e instanceof Error ? e.message : String(e) }) ?? String(e),
      );
    } finally {
      setPosting(false);
    }
  };

  return (
    <SplitDiff
      files={files}
      patch={patch}
      renderLineRight={
        canWrite
          ? ({ line, file }) => {
              const target = targetOf(file, line);
              if (!target) return null;
              const isActive =
                !!active &&
                commentKey(active.file, active.side, active.line) ===
                  commentKey(target.file, target.side, target.line);
              return (
                <button
                  type="button"
                  className="absolute right-1 top-0 hidden h-5 items-center gap-1 rounded bg-background px-1 text-[10px] text-muted-foreground shadow-sm ring-1 ring-border hover:text-foreground group-hover:flex"
                  title={t("diff.addComment")}
                  onClick={() => setActive(isActive ? null : target)}
                >
                  <MessageSquare className="h-3 w-3" />
                </button>
              );
            }
          : undefined
      }
      renderLineFooter={({ line, file }) => {
        const target = targetOf(file, line);
        if (!target) return null;
        const keys: string[] = [];
        if (line.new != null) keys.push(commentKey(file, "new", line.new));
        if (line.old != null) keys.push(commentKey(file, "old", line.old));
        const lineComments = keys.flatMap((k) => commentMap.get(k) ?? []);
        const isActive =
          !!active &&
          commentKey(active.file, active.side, active.line) ===
            commentKey(target.file, target.side, target.line);
        if (lineComments.length === 0 && !isActive) return null;
        return (
          <div className="my-1 ml-4 space-y-1.5 rounded-md border bg-muted/30 p-2 font-sans">
            {lineComments.map((c) => (
              <div key={c.id} className="text-xs">
                <span className="font-medium">{c.author}</span>
                <span className="ml-2 text-muted-foreground">
                  {formatDate(c.created_at, locale)}
                </span>
                <MarkdownView text={c.body} className="mt-0.5 text-xs leading-5" />
              </div>
            ))}
            {isActive && (
              <div className="space-y-1.5">
                <Textarea
                  rows={2}
                  placeholder={t("comments.placeholder")}
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                />
                <div className="flex gap-2">
                  <Button size="sm" disabled={posting || !body.trim()} onClick={post}>
                    {t("comments.post")}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setActive(null)}>
                    {t("common.cancel")}
                  </Button>
                </div>
              </div>
            )}
          </div>
        );
      }}
    />
  );
}

/** 简单 diff（无行内评论），供提交 diff 等场景复用 */
export function DiffView({ files, patch }: { files: DiffFileInfo[]; patch: string }) {
  return <SplitDiff files={files} patch={patch} />;
}

export function PatchOnly({ files, patch }: { files: DiffFileInfo[]; patch: string }) {
  return <SplitDiff files={files} patch={patch} />;
}
