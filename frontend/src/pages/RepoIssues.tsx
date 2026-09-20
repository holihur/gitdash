import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Flag, MessageSquare, Plus, Search, Tag } from "lucide-react";
import { api, type ByokKey, type Issue, type Label, type Milestone } from "@/lib/api";
import { useQueryState } from "@/lib/query-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import Pagination from "@/components/ui/pagination";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import ListSkeleton from "@/components/list-skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import LabelsManager from "@/components/labels-manager";
import MilestonesManager from "@/components/milestones-manager";
import { CreateIssueDialog, EditIssueDialog, CopilotLaunchDialog } from "@/components/issues/dialogs";
import { IssueItem, type IssueDraft } from "@/components/issues/issue-item";
import { IssueFilters } from "@/components/issues/issue-filters";

export default function RepoIssues({ owner, name, role }: { owner: string; name: string; role?: "owner" | "read" | "write" }) {
  const { t, to } = useI18n();
  const navigate = useNavigate();
  const canWrite = role === "owner" || role === "write";
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
  // 里程碑过滤："" 全部 | "none" 未指派 | 里程碑 id 字符串
  const milestoneFilter = get("i_milestone", "");
  const setMilestoneFilter = (v: string) =>
    set({ i_milestone: v || null, i_page: null }, { push: true });
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
  const [drafts, setDrafts] = useState<Record<number, IssueDraft>>({});
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

  // 用 Copilot 修复 issue
  const [copilotTarget, setCopilotTarget] = useState<Issue | null>(null);
  const [byokKeys, setByokKeys] = useState<ByokKey[] | null>(null);
  const [copilotByokId, setCopilotByokId] = useState(0);
  const [copilotNote, setCopilotNote] = useState("");
  const [copilotBusy, setCopilotBusy] = useState(false);

  const openCopilot = async (issue: Issue) => {
    setCopilotTarget(issue);
    setCopilotNote("");
    if (byokKeys === null) {
      try {
        const keys = await api.listByok();
        setByokKeys(keys);
        setCopilotByokId(keys[0]?.id ?? 0);
      } catch {
        setByokKeys([]);
      }
    } else {
      setCopilotByokId(byokKeys[0]?.id ?? 0);
    }
  };

  const launchCopilot = async () => {
    if (!copilotTarget || !copilotByokId) return;
    setCopilotBusy(true);
    try {
      const session = await api.createCopilot(owner, name, {
        byok_id: copilotByokId,
        issue_number: copilotTarget.number,
        prompt: copilotNote.trim(),
      });
      toast.success(t("copilot.launch"));
      setCopilotTarget(null);
      navigate(`/repo/${owner}/${name}/copilot?copilot=${session.id}`);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setCopilotBusy(false);
    }
  };

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [is, ls, ms] = await Promise.all([
        api.listIssues(owner, name, pageSize, (page - 1) * pageSize, {
          q: urlQuery,
          state: stateFilter,
          milestone: milestoneFilter,
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
  }, [owner, name, page, pageSize, urlQuery, stateFilter, milestoneFilter]);

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
          <CreateIssueDialog
            open={open}
            onOpenChange={setOpen}
            title={title}
            onTitleChange={setTitle}
            body={body}
            onBodyChange={setBody}
            busy={creating}
            onSubmit={create}
          />
        </div>
      </div>

      <IssueFilters
        searchInput={searchInput}
        onSearchInput={setSearchInput}
        stateFilter={stateFilter}
        onStateFilter={setStateFilter}
        labels={labels}
        filterLabel={filterLabel}
        onFilterLabel={setFilterLabel}
        milestones={milestones}
        filterMilestone={milestoneFilter}
        onFilterMilestone={setMilestoneFilter}
      />

      {error && !loading && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {t("issues.loadFailed", { error })}
          </CardContent>
        </Card>
      )}

      {loading && <ListSkeleton rows={5} header={false} />}

      {!loading && !error && issues.length === 0 && urlQuery === "" && stateFilter === "" && milestoneFilter === "" && (
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

      {!loading && !error && issues.length === 0 && filterLabel === null && (urlQuery !== "" || stateFilter !== "" || milestoneFilter !== "") && (
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
                setMilestoneFilter("");
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
            const busy = busyIds.has(issue.id);
            const openDetail = expanded === issue.number;
            const issueLabels = issue.labels ?? [];
            const draft = drafts[issue.number] ?? { labels: [], milestone: 0 };
            const metaChanged =
              draft.labels.length !== issueLabels.length ||
              draft.labels.some((id) => !issueLabels.some((l) => l.id === id)) ||
              (issue.milestone?.id ?? 0) !== draft.milestone;
            return (
              <IssueItem
                key={issue.id}
                issue={issue}
                openDetail={openDetail}
                busy={busy}
                draft={draft}
                labels={labels}
                milestones={milestones}
                canWrite={canWrite}
                savingMeta={savingMeta === issue.number}
                metaChanged={metaChanged}
                owner={owner}
                name={name}
                onToggleExpand={() => toggleExpand(issue)}
                onToggleState={() => setState(issue, issue.state === "open" ? "closed" : "open")}
                onTogglePin={() => togglePin(issue)}
                onEdit={() => openEdit(issue)}
                onFixWithCopilot={() => openCopilot(issue)}
                onDelete={() => setDeleteTarget(issue)}
                onToggleLabel={(id) => toggleDraftLabel(issue.number, id)}
                onSetMilestone={(id) => setDraftMilestone(issue.number, id)}
                onSaveMeta={() => saveMeta(issue)}
              />
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

      <EditIssueDialog
        open={editTarget !== null}
        number={editTarget?.number ?? 0}
        title={editTitle}
        onTitleChange={setEditTitle}
        body={editBody}
        onBodyChange={setEditBody}
        busy={savingEdit}
        onSubmit={saveEdit}
        onCancel={() => setEditTarget(null)}
      />

      <CopilotLaunchDialog
        open={copilotTarget !== null}
        number={copilotTarget?.number ?? 0}
        byokKeys={byokKeys}
        byokId={copilotByokId}
        onByokChange={setCopilotByokId}
        note={copilotNote}
        onNoteChange={setCopilotNote}
        busy={copilotBusy}
        onSubmit={launchCopilot}
        onCancel={() => setCopilotTarget(null)}
      />

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
