import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { MessageSquare } from "lucide-react";
import { api, type IssueComment } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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

/** 行内评论定位键 */
const commentKey = (file: string, side: "old" | "new", line: number) => `${file}|${side}|${line}`;

interface CommentTarget {
  file: string;
  line: number;
  side: "old" | "new";
}

/** PR diff 展示：文件统计徽标 + 逐行补丁 + 行级 hover 行内评论。 */
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

  const parsed = useMemo(() => parsePatch(patch), [patch]);

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

  const post = async () => {
    if (!active || !body.trim()) return;
    setPosting(true);
    try {
      await api.postComment(
        owner,
        name,
        number,
        body.trim(),
        "pulls",
        { file_path: active.file, line: active.line, line_side: active.side },
      );
      toast.success(t("comments.posted"));
      setBody("");
      setActive(null);
      load();
    } catch (e) {
      toast.error(to("comments.failed", { error: e instanceof Error ? e.message : String(e) }) ?? String(e));
    } finally {
      setPosting(false);
    }
  };

  return (
    <div className="space-y-2">
      {files.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {files.map((f) => (
            <Badge
              key={f.path}
              variant="outline"
              className="max-w-full font-mono text-xs"
              title={`${f.path} (+${f.insertions} -${f.deletions})`}
            >
              <span
                className={
                  f.status === "A"
                    ? "text-green-600 dark:text-green-400"
                    : f.status === "D"
                      ? "text-red-600 dark:text-red-400"
                      : "text-yellow-600 dark:text-yellow-400"
                }
              >
                {f.status}
              </span>
              <span className="mx-1 truncate">{f.path}</span>
              <span className="shrink-0 text-muted-foreground">
                +{f.insertions} -{f.deletions}
              </span>
            </Badge>
          ))}
        </div>
      )}
      {patch ? (
        <div className="max-h-[60vh] overflow-auto rounded-md border bg-background/60">
          <div className="min-w-max px-3 py-2 font-mono text-xs leading-5">
            {parsed.map((pl, i) => {
              const hasLine = pl.new != null || pl.old != null;
              const keys: string[] = [];
              if (pl.new != null) keys.push(commentKey(pl.file, "new", pl.new));
              if (pl.old != null) keys.push(commentKey(pl.file, "old", pl.old));
              const lineComments = keys.flatMap((k) => commentMap.get(k) ?? []);
              const target: CommentTarget | null =
                hasLine && pl.file
                  ? {
                      file: pl.file,
                      line: (pl.new ?? pl.old) as number,
                      side: pl.new != null ? "new" : "old",
                    }
                  : null;
              const isActive =
                !!active &&
                !!target &&
                commentKey(target.file, target.side, target.line) ===
                  commentKey(active.file, active.side, active.line);
              const showForm = isActive;
              return (
                <div key={i}>
                  <div
                    className={`group relative ${pl.cls}`}
                    title={pl.file && hasLine ? `${pl.file}:${pl.new ?? pl.old}` : undefined}
                  >
                    {pl.raw || " "}
                    {canWrite && target && (
                      <button
                        type="button"
                        className="absolute right-1 top-0 hidden h-5 items-center gap-1 rounded bg-background px-1 text-[10px] text-muted-foreground shadow-sm ring-1 ring-border hover:text-foreground group-hover:flex"
                        title={t("diff.addComment")}
                        onClick={() =>
                          setActive(
                            isActive
                              ? null
                              : { file: target.file, line: target.line, side: target.side },
                          )
                        }
                      >
                        <MessageSquare className="h-3 w-3" />
                      </button>
                    )}
                  </div>
                  {(lineComments.length > 0 || showForm) && (
                    <div className="my-1 ml-4 space-y-1.5 rounded-md border bg-muted/30 p-2 font-sans">
                      {lineComments.map((c) => (
                        <div key={c.id} className="text-xs">
                          <span className="font-medium">{c.author}</span>
                          <span className="ml-2 text-muted-foreground">
                            {formatDate(c.created_at, locale)}
                          </span>
                          <p className="mt-0.5 whitespace-pre-wrap break-words">{c.body}</p>
                        </div>
                      ))}
                      {showForm && (
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
                  )}
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">-</p>
      )}
    </div>
  );
}

/** 简单 diff（无行内评论），供提交 diff 等场景复用 */
export function DiffView({ files, patch }: { files: DiffFileInfo[]; patch: string }) {
  return <PatchOnly files={files} patch={patch} />;
}

export function PatchOnly({ files, patch }: { files: DiffFileInfo[]; patch: string }) {
  const lines = parsePatch(patch);
  return (
    <div className="space-y-2">
      {files.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {files.map((f) => (
            <Badge
              key={f.path}
              variant="outline"
              className="max-w-full font-mono text-xs"
              title={`${f.path} (+${f.insertions} -${f.deletions})`}
            >
              <span
                className={
                  f.status === "A"
                    ? "text-green-600 dark:text-green-400"
                    : f.status === "D"
                      ? "text-red-600 dark:text-red-400"
                      : "text-yellow-600 dark:text-yellow-400"
                }
              >
                {f.status}
              </span>
              <span className="mx-1 truncate">{f.path}</span>
              <span className="shrink-0 text-muted-foreground">
                +{f.insertions} -{f.deletions}
              </span>
            </Badge>
          ))}
        </div>
      )}
      {patch ? (
        <div className="max-h-[60vh] overflow-auto rounded-md border bg-background/60">
          <pre className="min-w-max px-3 py-2 font-mono text-xs leading-5">
            {lines.map((pl, i) => (
              <div key={i} className={pl.cls}>
                {pl.raw || " "}
              </div>
            ))}
          </pre>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">-</p>
      )}
    </div>
  );
}
