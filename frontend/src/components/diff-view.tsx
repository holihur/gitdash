import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { Check, MessageSquare } from "lucide-react";
import { api, type IssueComment } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { MarkdownView } from "@/components/markdown";
import { Textarea } from "@/components/ui/textarea";
import { useI18n } from "@/lib/i18n";
import { RelativeTime } from "@/components/relative-time";
import { apiErrorMsg } from "@/lib/errors";
import SplitDiff from "@/components/split-diff";
import {
  commentKey,
  type CommentTarget,
  type DiffFileInfo,
  type ParsedLine,
} from "@/components/diff-parse";

export type { DiffFileInfo } from "@/components/diff-parse";

/** 从评论正文提取首个 ```suggestion 代码块内容。 */
function parseSuggestion(body: string): string | null {
  const m = body.replace(/\r\n/g, "\n").match(/```suggestion[ \t]*\n([\s\S]*?)```/);
  if (!m) return null;
  return m[1].replace(/\n$/, "");
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
  const [applying, setApplying] = useState<number | null>(null);

  const load = useCallback(async () => {
    try {
      const all = await api.listComments(owner, name, number, "pulls");
      setComments((all ?? []).filter((c) => c.file_path && c.line));
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

  const apply = async (c: IssueComment) => {
    setApplying(c.id);
    try {
      await api.applySuggestion(owner, name, number, c.id);
      toast.success(t("diff.suggestionAppliedToast"));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setApplying(null);
    }
  };

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
            {lineComments.map((c) => {
              const suggestion = parseSuggestion(c.body);
              const applied = !!c.suggestion_applied_sha;
              return (
                <div key={c.id} className="text-xs">
                  <span className="font-medium">{c.author}</span>
                  <span className="ml-2 text-muted-foreground">
                    <RelativeTime iso={c.created_at} locale={locale} />
                  </span>
                  <MarkdownView text={c.body} className="mt-0.5 text-xs leading-5" />
                  {suggestion !== null && c.line_side !== "old" && (
                    <div className="mt-1">
                      {applied ? (
                        <span className="text-[11px] text-green-600 dark:text-green-400">
                          {t("diff.suggestionApplied", { sha: c.suggestion_applied_sha!.slice(0, 7) })}
                        </span>
                      ) : canWrite ? (
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-6 gap-1 text-[11px]"
                          disabled={applying === c.id}
                          onClick={() => apply(c)}
                        >
                          <Check className="h-3 w-3" />
                          {t("diff.applySuggestion")}
                        </Button>
                      ) : null}
                    </div>
                  )}
                </div>
              );
            })}
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
