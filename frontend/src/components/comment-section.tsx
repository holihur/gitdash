import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { MessageSquare, Pencil, Trash2 } from "lucide-react";
import { api, type IssueComment, type IssueEvent } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { MarkdownView } from "@/components/markdown";
import { MarkdownEditor } from "@/components/markdown-editor";
import { useI18n } from "@/lib/i18n";
import { RelativeTime } from "@/components/relative-time";

interface Props {
  owner: string;
  name: string;
  number: number;
  kind?: "issues" | "pulls";
  /** 可选：与评论交错展示的活动事件（仅 issue 传入）。 */
  events?: IssueEvent[];
}

const ACTION_KEYS: Record<string, string> = {
  opened: "issues.tlOpened",
  closed: "issues.tlClosed",
  reopened: "issues.tlReopened",
  edited: "issues.tlEdited",
  labeled: "issues.tlLabeled",
  unlabeled: "issues.tlUnlabeled",
  milestoned: "issues.tlMilestoned",
  demilestoned: "issues.tlDemilestoned",
  pinned: "issues.tlPinned",
  unpinned: "issues.tlUnpinned",
  assigned: "issues.tlAssigned",
  unassigned: "issues.tlUnassigned",
  commented: "issues.tlCommented",
};

export default function CommentSection({ owner, name, number, kind = "issues", events }: Props) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [comments, setComments] = useState<IssueComment[]>([]);
  const [loading, setLoading] = useState(true);
  const [me, setMe] = useState<string>("");
  const [body, setBody] = useState("");
  const [posting, setPosting] = useState(false);

  const [editingId, setEditingId] = useState<number | null>(null);
  const [editBody, setEditBody] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);

  const load = useCallback(async () => {
    try {
      const list = await api.listComments(owner, name, number, kind);
      setComments(list ?? []);
    } catch (e) {
      toast.error(to("comments.failed", { error: e instanceof Error ? e.message : String(e) }) ?? String(e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, number, kind, to]);

  useEffect(() => {
    setLoading(true);
    load();
  }, [load]);

  useEffect(() => {
    api
      .me()
      .then((u) => setMe(u.username))
      .catch(() => setMe(""));
  }, []);

  const post = async () => {
    if (!body.trim()) return;
    setPosting(true);
    try {
      await api.postComment(owner, name, number, body.trim(), kind);
      toast.success(t("comments.posted"));
      setBody("");
      load();
    } catch (e) {
      toast.error(to("comments.failed", { error: e instanceof Error ? e.message : String(e) }) ?? String(e));
    } finally {
      setPosting(false);
    }
  };

  const remove = async (c: IssueComment) => {
    try {
      await api.deleteComment(owner, name, c.id);
      toast.success(t("comments.deleted"));
      load();
    } catch (e) {
      toast.error(to("comments.failed", { error: e instanceof Error ? e.message : String(e) }) ?? String(e));
    }
  };

  const startEdit = (c: IssueComment) => {
    setEditingId(c.id);
    setEditBody(c.body);
  };

  const cancelEdit = () => {
    setEditingId(null);
    setEditBody("");
  };

  const saveEdit = async (c: IssueComment) => {
    const body = editBody.trim();
    if (!body) return;
    setSavingEdit(true);
    try {
      await api.updateComment(owner, name, c.id, body);
      toast.success(t("comments.posted"));
      cancelEdit();
      load();
    } catch (e) {
      toast.error(to("comments.failed", { error: e instanceof Error ? e.message : String(e) }) ?? String(e));
    } finally {
      setSavingEdit(false);
    }
  };

  const timeline = [
    ...comments.map((c) => ({ type: "comment" as const, at: c.created_at, c })),
    ...(events ?? [])
      .filter((e) => e.action !== "commented")
      .map((e) => ({ type: "event" as const, at: e.created_at, e })),
  ].sort((a, b) => (a.at < b.at ? -1 : a.at > b.at ? 1 : 0));

  return (
    <div className="space-y-3">
      <p className="text-xs font-medium text-muted-foreground">{t("comments.title")}</p>
      {loading ? (
        <p className="py-2 text-center text-xs text-muted-foreground">…</p>
      ) : timeline.length === 0 ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <MessageSquare className="h-3.5 w-3.5" />
          {t("comments.empty")}
        </p>
      ) : (
        <div className="space-y-2">
          {timeline.map((item) =>
            item.type === "event" ? (
              <div
                key={`e${item.e.id}`}
                className="flex flex-wrap items-center gap-x-2 gap-y-1 px-1 text-xs text-muted-foreground"
              >
                <span className="font-medium text-foreground">{item.e.actor}</span>
                <span>{t(ACTION_KEYS[item.e.action] ?? "issues.timeline")}</span>
                {item.e.detail && (
                  <code className="rounded bg-muted px-1.5 py-0.5 text-[11px]">{item.e.detail}</code>
                )}
                <RelativeTime iso={item.e.created_at} locale={locale} />
              </div>
            ) : (
              <div key={item.c.id} className="rounded-lg border bg-background p-3">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{item.c.author}</span>
                  <span className="text-xs text-muted-foreground">
                    <RelativeTime iso={item.c.created_at} locale={locale} />
                  </span>
                  {item.c.updated_at && item.c.updated_at !== item.c.created_at && (
                    <span className="text-xs italic text-muted-foreground">
                      ({t("issues.commentEdited")})
                    </span>
                  )}
                  {me && item.c.author === me && editingId !== item.c.id && (
                    <div className="ml-auto flex items-center">
                      <Button
                        size="icon"
                        variant="ghost"
                        className="h-7 w-7 text-muted-foreground"
                        title={t("issues.editComment")}
                        onClick={() => startEdit(item.c)}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        className="h-7 w-7 text-muted-foreground hover:text-destructive"
                        title={t("comments.delete")}
                        onClick={() => remove(item.c)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  )}
                </div>
                {editingId === item.c.id ? (
                  <div className="mt-2 space-y-2">
                    <MarkdownEditor rows={3} value={editBody} onChange={setEditBody} />
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        disabled={savingEdit || !editBody.trim()}
                        onClick={() => saveEdit(item.c)}
                      >
                        {t("issues.saveComment")}
                      </Button>
                      <Button size="sm" variant="outline" onClick={cancelEdit}>
                        {t("common.cancel")}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="mt-1">
                    <MarkdownView text={item.c.body} />
                  </div>
                )}
              </div>
            ),
          )}
        </div>
      )}
      <div className="space-y-2">
        <MarkdownEditor
          rows={3}
          placeholder={t("comments.placeholder")}
          value={body}
          onChange={setBody}
        />
        <Button size="sm" disabled={posting || !body.trim()} onClick={post}>
          {t("comments.post")}
        </Button>
      </div>
    </div>
  );
}
