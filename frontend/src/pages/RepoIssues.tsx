import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Flag, MessageSquare, Plus, Search, Tag } from "lucide-react";
import { api, type Issue, type Label, type Milestone } from "@/lib/api";
import { useQueryState } from "@/lib/query-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import Pagination from "@/components/ui/pagination";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import ListSkeleton from "@/components/list-skeleton";
import LabelsManager from "@/components/labels-manager";
import MilestonesManager from "@/components/milestones-manager";
import { CreateIssueDialog } from "@/components/issues/dialogs";
import { IssueItem } from "@/components/issues/issue-item";
import { canTriage, canWrite } from "@/lib/repo-role";
import { IssueFilters } from "@/components/issues/issue-filters";

export default function RepoIssues({ owner, name, role }: { owner: string; name: string; role?: "owner" | "read" | "write" }) {
  const { t, to } = useI18n();
  const canWriteCode = canWrite(role);
  const canManageIssues = canTriage(role);
  const [issues, setIssues] = useState<Issue[]>([]);
  const [issueTotal, setIssueTotal] = useState(0);
  // 页码/页大小/标签筛选同步进 URL(?i_page/?i_size/?i_label)
  const { get, getNum, set } = useQueryState();
  const page = getNum("i_page", 1);
  const setPage = (p: number) => set({ i_page: p > 1 ? p : null }, { push: true });
  const pageSize = getNum("i_size", 20);
  const setPageSize = (s: number) => set({ i_size: s === 20 ? null : s, i_page: null });
  const filterLabelRaw = get("i_label", "");
  const filterLabel = filterLabelRaw ? Number(filterLabelRaw) : null;
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
  const assigneeFilter = get("i_assignee", "");
  const setAssigneeFilter = (v: string) =>
    set({ i_assignee: v || null, i_page: null }, { push: true });
  const sortFilter = get("i_sort", "");
  const setSortFilter = (v: string) => set({ i_sort: v || null, i_page: null }, { push: true });
  const [searchInput, setSearchInput] = useState(urlQuery);
  const [counts, setCounts] = useState<{ open: number; closed: number }>({ open: 0, closed: 0 });
  const [labels, setLabels] = useState<Label[]>([]);
  const [milestones, setMilestones] = useState<Milestone[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

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
      const [is, ls, ms, cnt] = await Promise.all([
        api.listIssues(owner, name, pageSize, (page - 1) * pageSize, {
          q: urlQuery,
          state: stateFilter,
          milestone: milestoneFilter,
          label: filterLabelRaw || undefined,
          assignee: assigneeFilter || undefined,
          sort: sortFilter || undefined,
        }),
        api.listLabels(owner, name),
        api.listMilestones(owner, name),
        api.issueCounts(owner, name, {
          q: urlQuery || undefined,
          milestone: milestoneFilter || undefined,
          label: filterLabelRaw || undefined,
          assignee: assigneeFilter || undefined,
        }),
      ]);
      setIssues(is.items);
      setIssueTotal(is.total);
      setLabels(ls);
      setMilestones(ms);
      setCounts(cnt);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, page, pageSize, urlQuery, stateFilter, milestoneFilter, filterLabelRaw, assigneeFilter, sortFilter]);

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

  const openCount = counts.open;
  const closedCount = counts.closed;

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
        filterAssignee={assigneeFilter}
        onFilterAssignee={setAssigneeFilter}
        filterSort={sortFilter}
        onFilterSort={setSortFilter}
      />

      {error && !loading && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {t("issues.loadFailed", { error })}
          </CardContent>
        </Card>
      )}

      {loading && <ListSkeleton rows={5} header={false} />}

      {!loading && !error && issues.length === 0 && urlQuery === "" && stateFilter === "" && milestoneFilter === "" && filterLabel === null && assigneeFilter === "" && (
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

      {!loading && !error && issues.length === 0 && (urlQuery !== "" || stateFilter !== "" || milestoneFilter !== "" || filterLabel !== null || assigneeFilter !== "") && (
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
                setFilterLabel(null);
                setAssigneeFilter("");
              }}
            >
              {t("issues.clearSearch")}
            </Button>
          </CardContent>
        </Card>
      )}

      {!loading && !error && issues.length > 0 && (
        <div className="divide-y divide-border overflow-hidden rounded-lg border bg-card">
          {issues.map((issue) => (
            <IssueItem
              key={issue.id}
              issue={issue}
              owner={owner}
              name={name}
              canWrite={canWriteCode}
              canTriage={canManageIssues}
              onChanged={load}
            />
          ))}
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
