import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import {
  CheckCircle2,
  Circle,
  Flag,
  MessageSquare,
  Pencil,
  Pin,
  PinOff,
  Plus,
  Search,
  Tag,
  Trash2,
} from "lucide-react";
import { api, type Issue, type Label, type Milestone } from "@/lib/api";
import { useQueryState } from "@/lib/query-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label as FieldLabel } from "@/components/ui/label";
import { MarkdownEditor } from "@/components/markdown-editor";
import Pagination from "@/components/ui/pagination";
import { cn, formatDate } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import LabelChip from "@/components/label-chip";
import { MarkdownView } from "@/components/markdown";
import CommentSection from "@/components/comment-section";
import LabelsManager from "@/components/labels-manager";
import MilestonesManager from "@/components/milestones-manager";
import ListSkeleton from "@/components/list-skeleton";
import ConfirmDialog from "@/components/confirm-dialog";

interface Draft {
  labels: number[];
  milestone: number; // 0 = 无
}

export default function RepoIssues({ owner, name }: { owner: string; name: string }) {
  const { t, lang, to } = useI18n();
  const locale = dateLocale(lang);
  const [issues, setIssues] = useState<Issue[]>([]);
  const [issueTotal, setIssueTotal] = useState(0);
  // 页码/页大小/标签筛选同步进 URL(?i_page/?i_size/?i_label)
  const { get, getNum, set } = useQueryState();
  const page = getNum("i_page", 1);
  const setPage = (p: number) => set({ i_page: p > 1 ? p : null }, { push: true });
  const pageSize = getNum("i_size", 20);
  const setPageSize = (s: number) => set({ i_size: s === 20 ? null : s, i_page: null });
  const filterLabel = get("i_label", "") ? Number(get("i_label", "")) : null;
  const setFilterLabel = (id: number | null) =>
    set({ i_label: id ?? null, i_page: null }, { push: true });
  // 搜索词 / 状态过滤同步进 URL(?i_q/?i_state)
  const urlQuery = get("i_q", "");
  const stateFilter = get("i_state", ""); // "" | "open" | "closed"
  const setStateFilter = (s: string) =>
    set({ i_state: s || null, i_page: null }, { push: true });
  const [searchInput, setSearchInput] = useState(urlQuery);
  const [labels, setLabels] = useState<Label[]>([]);
  const [milestones, setMilestones] = useState<Milestone[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<number | null>(null);
  const [busyIds, setBusyIds] = useState<Set<number>>(new Set());
  const [drafts, setDrafts] = useState<Record<number, Draft>>({});
  const [savingMeta, setSavingMeta] = useState<number | null>(null);

  // 管理对话框
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [msOpen, setMsOpen] = useState(false);

  // 新建 issue
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [creating, setCreating] = useState(false);

  // 编辑 / 删除 issue
  const [editTarget, setEditTarget] = useState<Issue | null>(null);
  const [editTitle, setEditTitle] = useState("");
  const [editBody, setEditBody] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Issue | null>(null);
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [is, ls, ms] = await Promise.all([
        api.listIssues(owner, name, pageSize, (page - 1) * pageSize, {
          q: urlQuery,
          state: stateFilter,
        }),
        api.listLabels(owner, name),
        api.listMilestones(owner, name),
      ]);
      setIssues(is.items);
      setIssueTotal(is.total);
      setLabels(ls);
      setMilestones(ms);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, page, pageSize, urlQuery, stateFilter]);

  useEffect(() => {
    load();
  }, [load]);

  // 输入防抖：停止输入 300ms 后写入 URL 触发搜索
  useEffect(() => {
    if (searchInput.trim() === urlQuery) return;
    const timer = setTimeout(() => {
      set({ i_q: searchInput.trim() || null, i_page: null }, { push: false });
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput, urlQuery, set]);

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

  const openEdit = (issue: Issue) => {
    setEditTarget(issue);
    setEditTitle(issue.title);
    setEditBody(issue.body);
  };

  const saveEdit = async () => {
    if (!editTarget || !editTitle.trim()) return;
    const patch: { title?: string; body?: string } = {};
    if (editTitle.trim() !== editTarget.title) patch.title = editTitle.trim();
    if (editBody !== editTarget.body) patch.body = editBody;
    if (Object.keys(patch).length === 0) {
      setEditTarget(null);
      return;
    }
    setSavingEdit(true);
    try {
      await api.updateIssue(owner, name, editTarget.number, patch);
      toast.success(t("issues.edited", { number: editTarget.number }));
      setEditTarget(null);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSavingEdit(false);
    }
  };

  const togglePin = async (issue: Issue) => {
    setBusyIds((s) => new Set(s).add(issue.id));
    try {
      await api.updateIssue(owner, name, issue.number, { pinned: !issue.pinned });
      toast.success(
        issue.pinned
          ? t("issues.unpinned", { number: issue.number })
          : t("issues.pinnedMsg", { number: issue.number }),
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

  const removeIssue = async (issue: Issue) => {
    setDeleting(true);
    try {
      await api.deleteIssue(owner, name, issue.number);
      toast.success(t("issues.deleted", { number: issue.number }));
      setDeleteTarget(null);
      if (expanded === issue.number) setExpanded(null);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setDeleting(false);
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
          {/* 列表非空才显示头部按钮，避免与空状态 CTA 重复 */}
          {issues.length > 0 && (
            <Button size="sm" className="gap-1.5" onClick={() => setOpen(true)}>
              <Plus className="h-4 w-4" />
              {t("issues.new")}
            </Button>
          )}
          <Dialog open={open} onOpenChange={setOpen}>
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
                  <MarkdownEditor
                    id="issue-body"
                    rows={5}
                    placeholder={t("issues.bodyPlaceholder")}
                    value={body}
                    onChange={setBody}
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

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            placeholder={t("issues.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
        </div>
        <div className="flex items-center gap-1">
          {(["", "open", "closed"] as const).map((s) => (
            <Button
              key={s || "all"}
              size="sm"
              variant={stateFilter === s ? "secondary" : "outline"}
              onClick={() => setStateFilter(s)}
            >
              {s === ""
                ? t("issues.filterAll")
                : s === "open"
                  ? t("issues.open")
                  : t("issues.closed")}
            </Button>
          ))}
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

      {loading && <ListSkeleton rows={5} header={false} />}

      {!loading && !error && issues.length === 0 && urlQuery === "" && stateFilter === "" && (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
            <MessageSquare className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("issues.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("issues.emptyHint")}</p>
            <Button size="sm" className="gap-1.5" onClick={() => setOpen(true)}>
              <Plus className="h-4 w-4" />
              {t("issues.new")}
            </Button>
          </CardContent>
        </Card>
      )}

      {!loading && !error && issues.length === 0 && filterLabel === null && (urlQuery !== "" || stateFilter !== "") && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-10 text-center">
            <Search className="h-8 w-8 text-muted-foreground" />
            <p className="font-medium">{t("issues.noSearchMatch")}</p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setSearchInput("");
                setStateFilter("");
              }}
            >
              {t("issues.clearSearch")}
            </Button>
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
                      <span className="flex items-center gap-1">
                        {issue.pinned && (
                          <Pin className="h-3.5 w-3.5 shrink-0 fill-current text-amber-500" />
                        )}
                        <span
                          className={cn(
                            "min-w-0 truncate font-medium hover:underline",
                            !isOpen && "text-muted-foreground",
                          )}
                        >
                          {issue.title}
                        </span>
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
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 w-8 shrink-0 p-0"
                      title={issue.pinned ? t("issues.unpin") : t("issues.pin")}
                      disabled={busy}
                      onClick={() => togglePin(issue)}
                    >
                      {issue.pinned ? (
                        <PinOff className="h-3.5 w-3.5" />
                      ) : (
                        <Pin className="h-3.5 w-3.5" />
                      )}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 w-8 shrink-0 p-0"
                      title={t("issues.edit")}
                      onClick={() => openEdit(issue)}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 w-8 shrink-0 p-0 text-destructive"
                      title={t("issues.delete")}
                      onClick={() => setDeleteTarget(issue)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
                {openDetail && (
                  <div className="space-y-3 border-t bg-muted/30 px-4 py-3">
                    {issue.body.trim() ? (
                      <MarkdownView text={issue.body} />
                    ) : (
                      <p className="text-sm text-muted-foreground">{t("issues.noBody")}</p>
                    )}
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
                    <CommentSection owner={owner} name={name} number={issue.number} />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {!loading && !error && issueTotal > 0 && (
        <Pagination
          page={page}
          pageSize={pageSize}
          total={issueTotal}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
      )}

      <Dialog open={editTarget !== null} onOpenChange={(o) => !o && setEditTarget(null)}>
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {t("issues.editDialogTitle", { number: editTarget?.number ?? 0 })}
            </DialogTitle>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <FieldLabel htmlFor="edit-issue-title">{t("issues.titleLabel")}</FieldLabel>
              <Input
                id="edit-issue-title"
                maxLength={200}
                value={editTitle}
                onChange={(e) => setEditTitle(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <FieldLabel htmlFor="edit-issue-body">{t("issues.bodyLabel")}</FieldLabel>
              <MarkdownEditor
                id="edit-issue-body"
                rows={6}
                value={editBody}
                onChange={setEditBody}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={saveEdit} disabled={savingEdit || !editTitle.trim()}>
              {t("issues.saveEdit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
        title={t("issues.deleteConfirmTitle")}
        description={t("issues.deleteConfirmDesc", { number: deleteTarget?.number ?? 0 })}
        confirmText={t("common.delete")}
        busy={deleting}
        onConfirm={() => deleteTarget && removeIssue(deleteTarget)}
      />

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
