import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, GitPullRequestDraft, X } from "lucide-react";
import { api, type PullReview } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** PR review 区块：汇总徽章 + 历史 + 提交 review（需写权限）。 */
export default function PullReviewSection({
  owner,
  name,
  number,
  canWrite,
}: {
  owner: string;
  name: string;
  number: number;
  canWrite: boolean;
}) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [reviews, setReviews] = useState<PullReview[]>([]);
  const [summary, setSummary] = useState({ approvals: 0, request_changes: 0 });
  const [loading, setLoading] = useState(true);
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await api.listPullReviews(owner, name, number);
      setReviews(r.reviews);
      setSummary(r.summary);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, number, to]);

  useEffect(() => {
    load();
  }, [load]);

  const submit = async (state: "approve" | "request_changes" | "comment") => {
    setBusy(true);
    try {
      await api.createPullReview(owner, name, number, state, body.trim() || undefined);
      toast.success(t(state === "approve" ? "pulls.reviewApproved" : "pulls.reviewSubmitted"));
      setBody("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-2 rounded-lg border bg-background p-3">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-xs font-medium text-muted-foreground">{t("pulls.reviews")}</p>
        <Badge variant="outline" className="gap-1 text-green-600 dark:text-green-400">
          <Check className="h-3 w-3" />
          {t("pulls.approvals", { count: summary.approvals })}
        </Badge>
        <Badge variant="outline" className="gap-1 text-red-600 dark:text-red-400">
          <X className="h-3 w-3" />
          {t("pulls.changesRequested", { count: summary.request_changes })}
        </Badge>
      </div>

      {loading ? (
        <p className="py-2 text-center text-xs text-muted-foreground">…</p>
      ) : reviews.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("pulls.noReviews")}</p>
      ) : (
        <div className="space-y-1.5">
          {reviews.map((r) => (
            <div key={r.id} className="rounded-md border bg-muted/20 p-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{r.reviewer}</span>
                {r.state === "approve" ? (
                  <Badge variant="outline" className="gap-1 text-green-600 dark:text-green-400">
                    <Check className="h-3 w-3" />
                    {t("pulls.reviewApprovedLabel")}
                  </Badge>
                ) : r.state === "request_changes" ? (
                  <Badge variant="outline" className="gap-1 text-red-600 dark:text-red-400">
                    <X className="h-3 w-3" />
                    {t("pulls.reviewChangesLabel")}
                  </Badge>
                ) : (
                  <Badge variant="outline" className="gap-1">
                    <GitPullRequestDraft className="h-3 w-3" />
                    {t("pulls.reviewCommentLabel")}
                  </Badge>
                )}
                <span className="text-xs text-muted-foreground">
                  {formatDate(r.created_at, locale)}
                </span>
              </div>
              {r.body && <p className="mt-1 whitespace-pre-wrap break-words text-xs">{r.body}</p>}
            </div>
          ))}
        </div>
      )}

      {canWrite && (
        <div className="space-y-2 border-t pt-2">
          <Textarea
            rows={2}
            placeholder={t("pulls.reviewPlaceholder")}
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" className="gap-1 text-green-600 dark:text-green-400" disabled={busy} onClick={() => submit("approve")}>
              <Check className="h-3.5 w-3.5" />
              {t("pulls.approve")}
            </Button>
            <Button size="sm" variant="outline" className="gap-1 text-red-600 dark:text-red-400" disabled={busy} onClick={() => submit("request_changes")}>
              <X className="h-3.5 w-3.5" />
              {t("pulls.requestChanges")}
            </Button>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => submit("comment")}>
              {t("comments.post")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
