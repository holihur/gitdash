import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { MessageSquare, Trash2 } from "lucide-react";
import { api, type IssueComment } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

interface Props {
  owner: string;
  name: string;
  number: number;
  kind?: "issues" | "pulls";
}

export default function CommentSection({ owner, name, number, kind = "issues" }: Props) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [comments, setComments] = useState<IssueComment[]>([]);
  const [loading, setLoading] = useState(true);
  const [me, setMe] = useState<string>("");
  const [body, setBody] = useState("");
  const [posting, setPosting] = useState(false);

  const load = useCallback(async () => {
    try {
      setComments(await api.listComments(owner, name, number, kind));
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

  return (
    <div className="space-y-3">
      <p className="text-xs font-medium text-muted-foreground">{t("comments.title")}</p>
      {loading ? (
        <p className="py-2 text-center text-xs text-muted-foreground">…</p>
      ) : comments.length === 0 ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <MessageSquare className="h-3.5 w-3.5" />
          {t("comments.empty")}
        </p>
      ) : (
        <div className="space-y-2">
          {comments.map((c) => (
            <div key={c.id} className="rounded-lg border bg-background p-3">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium">{c.author}</span>
                <span className="text-xs text-muted-foreground">{formatDate(c.created_at, locale)}</span>
                {me && c.author === me && (
                  <Button
                    size="icon"
                    variant="ghost"
                    className="ml-auto h-7 w-7 text-muted-foreground hover:text-destructive"
                    title={t("comments.delete")}
                    onClick={() => remove(c)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
              <p className="mt-1 whitespace-pre-wrap break-words text-sm">{c.body}</p>
            </div>
          ))}
        </div>
      )}
      <div className="space-y-2">
        <Textarea
          rows={3}
          placeholder={t("comments.placeholder")}
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
        <Button size="sm" disabled={posting || !body.trim()} onClick={post}>
          {t("comments.post")}
        </Button>
      </div>
    </div>
  );
}
