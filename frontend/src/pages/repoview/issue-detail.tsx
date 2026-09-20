import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { CheckCircle2, ChevronLeft, Circle, Pin } from "lucide-react";
import { api, type Issue, type Label, type Milestone } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { dateLocale, useI18n } from "@/lib/i18n";
import { buildRepoPath } from "@/lib/repo-url";
import { cn, formatDate } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import LabelChip from "@/components/label-chip";
import { MarkdownView } from "@/components/markdown";
import CommentSection from "@/components/comment-section";
import { IssueActions } from "@/components/issues/issue-actions";

/** Issue 详情独立页：左侧正文 + 评论，右侧标签 / 里程碑设置。 */
export default function IssueDetail({
  owner,
  name,
  role,
  number,
}: {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
  number: number;
}) {
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
  const navigate = useNavigate();
  const canWrite = role === "owner" || role === "write";
  const listUrl = buildRepoPath(owner, name, { tab: "issues" });

  const [issue, setIssue] = useState<Issue | null>(null);
  const [labels, setLabels] = useState<Label[]>([]);
  const [milestones, setMilestones] = useState<Milestone[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [draftLabels, setDraftLabels] = useState<number[]>([]);
  const [draftMilestone, setDraftMilestone] = useState(0);
  const [savingMeta, setSavingMeta] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [it, ls, ms] = await Promise.all([
        api.getIssue(owner, name, number),
        api.listLabels(owner, name),
        api.listMilestones(owner, name),
      ]);
      setIssue(it);
      setLabels(ls);
      setMilestones(ms);
      setDraftLabels((it.labels ?? []).map((l) => l.id));
      setDraftMilestone(it.milestone?.id ?? 0);
      setError("");
    } catch (e) {
      setError(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, number, to]);

  useEffect(() => {
    load();
  }, [load]);

  const isOpen = issue?.state === "open";
  const issueLabels = issue?.labels ?? [];
  const metaChanged = issue
    ? draftLabels.length !== issueLabels.length ||
      draftLabels.some((id) => !issueLabels.some((l) => l.id === id)) ||
      (issue.milestone?.id ?? 0) !== draftMilestone
    : false;

  const toggleDraftLabel = (id: number) => {
    setDraftLabels((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]));
  };

  const saveMeta = async () => {
    if (!issue) return;
    setSavingMeta(true);
    try {
      await api.setIssueLabels(owner, name, issue.number, draftLabels);
      await api.setIssueMilestone(owner, name, issue.number, draftMilestone);
      toast.success(t("issues.metaSaved", { number: issue.number }));
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSavingMeta(false);
    }
  };

  const backLink = (
    <Button asChild variant="ghost" size="sm" className="-ml-2 gap-1.5">
      <Link to={listUrl}>
        <ChevronLeft className="h-4 w-4" />
        {t("issues.backToList")}
      </Link>
    </Button>
  );

  if (loading) {
    return (
      <div className="space-y-4">
        {backLink}
        <Skeleton className="h-7 w-2/3" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (error || !issue) {
    return (
      <div className="space-y-4">
        {backLink}
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {error || t("issues.notFound")}
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {backLink}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1.5">
          <h1 className="flex items-start gap-2 text-xl font-semibold">
            {issue.pinned && (
              <Pin className="mt-1 h-4 w-4 shrink-0 fill-current text-amber-500" />
            )}
            <span className="min-w-0 break-words">{issue.title}</span>
            <span className="shrink-0 text-muted-foreground">#{issue.number}</span>
          </h1>
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <Badge variant={isOpen ? "default" : "secondary"} className="gap-1">
              {isOpen ? (
                <Circle className="h-3 w-3 fill-current" />
              ) : (
                <CheckCircle2 className="h-3 w-3" />
              )}
              {isOpen ? t("issues.open") : t("issues.closed")}
            </Badge>
            <span>
              {isOpen
                ? t("issues.openedOn", {
                    author: issue.author,
                    date: formatDate(issue.created_at, locale),
                  })
                : t("issues.closedOn", {
                    author: issue.author,
                    date: formatDate(issue.closed_at ?? issue.updated_at, locale),
                  })}
            </span>
          </div>
        </div>
        <IssueActions
          owner={owner}
          name={name}
          issue={issue}
          canWrite={canWrite}
          onChanged={load}
          onDeleted={() => navigate(listUrl)}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_17rem]">
        <div className="min-w-0 space-y-4">
          <div className="rounded-lg border bg-card p-4">
            {issue.body.trim() ? (
              <MarkdownView text={issue.body} />
            ) : (
              <p className="text-sm text-muted-foreground">{t("issues.noBody")}</p>
            )}
          </div>
          <CommentSection owner={owner} name={name} number={issue.number} />
        </div>

        <aside className="self-start">
          <div className="space-y-4 rounded-lg border bg-card p-3">
            <div>
              <p className="mb-2 text-xs font-medium text-muted-foreground">
                {t("issues.labels")}
              </p>
              {labels.length === 0 ? (
                <p className="text-xs text-muted-foreground">{t("labels.emptyHint")}</p>
              ) : (
                <div className="flex flex-wrap gap-1.5">
                  {labels.map((l) => {
                    const selected = draftLabels.includes(l.id);
                    return (
                      <button
                        key={l.id}
                        type="button"
                        onClick={() => toggleDraftLabel(l.id)}
                        className={cn(
                          "rounded-full outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring",
                          selected ? "ring-2 ring-ring ring-offset-1" : "opacity-50 hover:opacity-80",
                        )}
                      >
                        <LabelChip label={l} />
                      </button>
                    );
                  })}
                </div>
              )}
            </div>

            <div className="space-y-1.5">
              <p className="text-xs font-medium text-muted-foreground">
                {t("issues.milestone")}
              </p>
              <select
                aria-label={t("issues.milestone")}
                value={draftMilestone || ""}
                onChange={(e) => setDraftMilestone(Number(e.target.value) || 0)}
                className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="">{t("issues.noMilestone")}</option>
                {milestones.map((m) => (
                  <option key={m.id} value={m.id} disabled={m.state === "closed"}>
                    {m.title}
                    {m.state === "closed" ? ` · ${t("issues.closed")}` : ""}
                  </option>
                ))}
              </select>
            </div>

            <Button
              size="sm"
              className="w-full"
              disabled={savingMeta || !metaChanged}
              onClick={saveMeta}
            >
              {t("issues.saveMeta")}
            </Button>
          </div>
        </aside>
      </div>
    </div>
  );
}
