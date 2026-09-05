import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import {
  CheckCircle2,
  Circle,
  Flag,
  MessageSquare,
  Plus,
  Tag,
} from "lucide-react";
import { api, type Issue, type Label, type Milestone } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label as FieldLabel } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { cn, formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import LabelChip from "@/components/label-chip";
import LabelsManager from "@/components/labels-manager";
import MilestonesManager from "@/components/milestones-manager";

interface Draft {
  labels: number[];
  milestone: number; // 0 = 无
}

export default function RepoIssues({ owner, name }: { owner: string; name: string }) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [issues, setIssues] = useState<Issue[]>([]);
  const [labels, setLabels] = useState<Label[]>([]);
  const [milestones, setMilestones] = useState<Milestone[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<number | null>(null);
  const [busyIds, setBusyIds] = useState<Set<number>>(new Set());
  const [drafts, setDrafts] = useState<Record<number, Draft>>({});
  const [savingMeta, setSavingMeta] = useState<number | null>(null);
  // 按标签筛选（client-side）
  const [filterLabel, setFilterLabel] = useState<number | null>(null);

  // 管理对话框
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [msOpen, setMsOpen] = useState(false);

  // 新建 issue
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [is, ls, ms] = await Promise.all([
        api.listIssues(owner, name),
        api.listLabels(owner, name),
        api.listMilestones(owner, name),
      ]);
      setIssues(is);
      setLabels(ls);
      setMilestones(ms);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [owner, name]);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    if (!title.trim()) return;
    setCreating(true);
    try {
      const it = await api.createIssue(owner, name, title.trim(), body.trim());
      toast.success(t("issues.created", { number: it.number }));
      setOpen(false);
      setTitle("");
      setBody("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setCreating(false);
    }
  };

  const setState = async (issue: Issue, state: "open" | "closed") => {
    setBusyIds((s) => new Set(s).add(issue.id));
    try {
      await api.setIssueState(owner, name, issue.number, state);
      toast.success(
        state === "closed"
          ? t("issues.stateClosed", { number: issue.number })
          : t("issues.stateOpen", { number: issue.number }),
      );
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusyIds((s) => {
        const n = new Set(s);
        n.delete(issue.id);
        return n;
      });
    }
  };

  const toggleExpand = (issue: Issue) => {
    const next = expanded === issue.number ? null : issue.number;
    setExpanded(next);
    if (next !== null && !drafts[issue.number]) {
      setDrafts((d) => ({
        ...d,
        [issue.number]: {
          labels: (issue.labels ?? []).map((l) => l.id),
          milestone: issue.milestone?.id ?? 0,
        },
      }));
    }
  };

  const toggleDraftLabel = (num: number, id: number) => {
    setDrafts((d) => {
      const cur = d[num] ?? { labels: [], milestone: 0 };
      const has = cur.labels.includes(id);
      return {
        ...d,
        [num]: {
          ...cur,
          labels: has ? cur.labels.filter((x) => x !== id) : [...cur.labels, id],
        },
      };
    });
  };

  const setDraftMilestone = (num: number, id: number) => {
    setDrafts((d) => ({
      ...d,
      [num]: { ...(d[num] ?? { labels: [], milestone: 0 }), milestone: id },
    }));
  };

  const saveMeta = async (issue: Issue) => {
    const draft = drafts[issue.number] ?? { labels: [], milestone: 0 };
    setSavingMeta(issue.number);
    try {
      await api.setIssueLabels(owner, name, issue.number, draft.labels);
      await api.setIssueMilestone(owner, name, issue.number, draft.milestone);
      toast.success(t("issues.metaSaved", { number: issue.number }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSavingMeta(null);
    }
  };

  const openCount = issues.filter((i) => i.state === "open").length;
  const closedCount = issues.length - openCount;
  const shown = filterLabel
    ? issues.filter((i) => (i.labels ?? []).some((l) => l.id === filterLabel))
    : issues;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          <span className="font-medium text-foreground">{openCount}</span> {t("issues.open")} ·{" "}
          <span className="font-medium text-foreground">{closedCount}</span> {t("issues.closed")}
        </p>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" className="gap-1.5" onClick={() => setLabelsOpen(true)}>
            <Tag className="h-4 w-4" />
            {t("issues.labels")}
          </Button>
          <Button size="sm" variant="outline" className="gap-1.5" onClick={() => setMsOpen(true)}>
            <Flag className="h-4 w-4" />
            {t("issues.milestones")}
          </Button>
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button size="sm" className="gap-2">
                <Plus className="h-4 w-4" />
                {t("issues.new")}
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>{t("issues.newDialogTitle")}</DialogTitle>
              </DialogHeader>
              <div className="grid gap-4">
                <div className="grid gap-2">
                  <FieldLabel htmlFor="issue-title">{t("issues.titleLabel")}</FieldLabel>
                  <Input
                    id="issue-title"
                    placeholder={t("issues.titlePlaceholder")}
                    maxLength={200}
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        create();
                      }
                    }}
                  />
                </div>
                <div className="grid gap-2">
                  <FieldLabel htmlFor="issue-body">{t("issues.bodyLabel")}</FieldLabel>
                  <Textarea
                    id="issue-body"
                    rows={5}
                    placeholder={t("issues.bodyPlaceholder")}
                    value={body}
                    onChange={(e) => setBody(e.target.value)}
                  />
                </div>
              </div>
              <DialogFooter>
                <Button onClick={create} disabled={creating || !title.trim()}>
                  {t("issues.new")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </div>

      {labels.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs text-muted-foreground">{t("issues.filterHint")}</span>
          {labels.map((l) => (
            <button
              key={l.id}
              type="button"
              onClick={() => setFilterLabel(filterLabel === l.id ? null : l.id)}
              className={cn(
                "rounded-full outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring",
                filterLabel === l.id ? "ring-2 ring-ring ring-offset-1" : "opacity-70 hover:opacity-100",
              )}
              title={l.name}
            >
              <LabelChip label={l} />
            </button>
          ))}
        </div>
      )}

      {error && !loading && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {t("issues.loadFailed", { error })}
          </CardContent>
        </Card>
      )}

      {loading && <p className="py-10 text-center text-sm text-muted-foreground">…</p>}

      {!loading && !error && issues.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <MessageSquare className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("issues.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("issues.emptyHint")}</p>
          </CardContent>
        </Card>
      )}

      {!loading && !error && shown.length === 0 && filterLabel !== null && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-10 text-center">
            <Tag className="h-8 w-8 text-muted-foreground" />
            <p className="font-medium">{t("issues.noMatch")}</p>
            <Button variant="outline" size="sm" onClick={() => setFilterLabel(null)}>
              {t("issues.clearFilter")}
            </Button>
          </CardContent>
        </Card>
      )}

      {!loading && shown.length > 0 && (
        <div className="divide-y divide-border overflow-hidden rounded-lg border bg-card">
          {shown.map((issue) => {
            const isOpen = issue.state === "open";
            const busy = busyIds.has(issue.id);
            const openDetail = expanded === issue.number;
            const issueLabels = issue.labels ?? [];
            const draft = drafts[issue.number] ?? { labels: [], milestone: 0 };
            const metaChanged =
              draft.labels.length !== issueLabels.length ||
              draft.labels.some((id) => !issueLabels.some((l) => l.id === id)) ||
              (issue.milestone?.id ?? 0) !== draft.milestone;
            return (
              <div key={issue.id}>
                <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:gap-3">
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-start gap-2 text-left"
                    onClick={() => toggleExpand(issue)}
                  >
                    {isOpen ? (
                      <Circle className="mt-0.5 h-4 w-4 shrink-0 fill-green-500 text-green-600" />
                    ) : (
                      <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                    )}
                    <span className="min-w-0 flex-1">
                      <span
                        className={cn(
                          "block truncate font-medium hover:underline",
                          !isOpen && "text-muted-foreground",
                        )}
                      >
                        {issue.title}
                      </span>
                      {(issueLabels.length > 0 || issue.milestone) && (
                        <span className="mt-1 flex flex-wrap items-center gap-1">
                          {issueLabels.map((l) => (
                            <LabelChip key={l.id} label={l} />
                          ))}
                          {issue.milestone && (
                            <span
                              className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs text-muted-foreground"
                              title={issue.milestone.description || issue.milestone.title}
                            >
                              <Flag className="h-3 w-3" />
                              <span className="max-w-40 truncate">{issue.milestone.title}</span>
                            </span>
                          )}
                        </span>
                      )}
                      <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                        #{issue.number} ·{" "}
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
                    </span>
                  </button>
                  <div className="flex shrink-0 items-center gap-2 pl-6 sm:pl-0">
                    <Button
                      size="sm"
                      variant="outline"
                      className="shrink-0"
                      disabled={busy}
                      onClick={() => setState(issue, isOpen ? "closed" : "open")}
                    >
                      {isOpen ? t("issues.close") : t("issues.reopen")}
                    </Button>
                  </div>
                </div>
                {openDetail && (
                  <div className="space-y-3 border-t bg-muted/30 px-4 py-3">
                    <p className="whitespace-pre-wrap break-words text-sm">
                      {issue.body.trim() || t("issues.noBody")}
                    </p>
                    <div className="space-y-3 rounded-lg border bg-card p-3">
                      <div>
                        <p className="mb-1.5 text-xs font-medium text-muted-foreground">
                          {t("issues.labels")}
                        </p>
                        {labels.length === 0 ? (
                          <p className="text-xs text-muted-foreground">{t("labels.emptyHint")}</p>
                        ) : (
                          <div className="flex flex-wrap gap-1.5">
                            {labels.map((l) => {
                              const selected = draft.labels.includes(l.id);
                              return (
                                <button
                                  key={l.id}
                                  type="button"
                                  onClick={() => toggleDraftLabel(issue.number, l.id)}
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
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-xs font-medium text-muted-foreground">
                          {t("issues.milestone")}
                        </p>
                        <select
                          value={draft.milestone || ""}
                          onChange={(e) => setDraftMilestone(issue.number, Number(e.target.value) || 0)}
                          className="h-9 flex-1 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                          <option value="">{t("issues.noMilestone")}</option>
                          {milestones.map((m) => (
                            <option key={m.id} value={m.id} disabled={m.state === "closed"}>
                              {m.title}
                              {m.state === "closed" ? ` · ${t("issues.closed")}` : ""}
                            </option>
                          ))}
                        </select>
                        <Button
                          size="sm"
                          disabled={savingMeta === issue.number || !metaChanged}
                          onClick={() => saveMeta(issue)}
                        >
                          {t("issues.saveMeta")}
                        </Button>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      <LabelsManager
        open={labelsOpen}
        onOpenChange={setLabelsOpen}
        owner={owner}
        repo={name}
        onChanged={load}
      />
      <MilestonesManager
        open={msOpen}
        onOpenChange={setMsOpen}
        owner={owner}
        repo={name}
        onChanged={load}
      />
    </div>
  );
}
