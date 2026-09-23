import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ArrowLeft, GanttChartSquare, KanbanSquare, Layers, List, Pencil, Plus, Search, SquarePlus } from "lucide-react";
import { api, type Issue, type Label, type Project, type ProjectBoard, type ProjectCard } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useQueryState } from "@/lib/query-state";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import ConfirmDialog from "@/components/confirm-dialog";
import { ProjectCardDialog, ProjectGanttView, ProjectListView, type CardDraft } from "@/components/projects-views";
import { ProjectBoardView, UNGROUPED } from "@/components/projects-board-view";

interface Props {
  owner: string;
  name: string;
  project: Project;
  role?: "owner" | "read" | "write";
  onBack: () => void;
  onProjectChanged: (p: Project) => void;
}

type ProjectView = "board" | "list" | "gantt";

export default function ProjectsBoard({ owner, name, project, role, onBack, onProjectChanged }: Props) {
  const { t, to } = useI18n();
  const canWrite = role === "owner" || role === "write";
  const { get, set: setQuery } = useQueryState();
  const [board, setBoard] = useState<ProjectBoard | null>(null);
  const [busy, setBusy] = useState(false);
  // 视图与筛选同步进 URL（刷新 / 分享可恢复）。
  const viewParam = get("p_view", "");
  const view: ProjectView = viewParam === "list" || viewParam === "gantt" ? viewParam : "board";
  const setView = (v: ProjectView) => setQuery({ p_view: v === "board" ? null : v });
  const [editCard, setEditCard] = useState<ProjectCard | null>(null);

  const [editProjectOpen, setEditProjectOpen] = useState(false);
  const [nameInput, setNameInput] = useState(project.name);
  const [descInput, setDescInput] = useState(project.description);

  const [newColumn, setNewColumn] = useState("");
  const [newLane, setNewLane] = useState("");
  const [columnDialogOpen, setColumnDialogOpen] = useState(false);
  const [laneDialogOpen, setLaneDialogOpen] = useState(false);
  const [pendingDeleteColumn, setPendingDeleteColumn] = useState<{ id: number; count: number } | null>(null);
  const [deleteColumnMoveTo, setDeleteColumnMoveTo] = useState<number>(0);
  const [pendingDeleteLane, setPendingDeleteLane] = useState<number | null>(null);
  // 新建卡片目标单元格（点击单元格内的“添加卡片”后打开对话框）
  const [addTarget, setAddTarget] = useState<{ swimlaneId: number; columnId: number } | null>(null);

  // 卡片编辑所需的仓库元数据（负责人候选、标签、可关联 issue）与当前用户。
  const [people, setPeople] = useState<string[]>([]);
  const [labels, setLabels] = useState<Label[]>([]);
  const [issues, setIssues] = useState<Issue[]>([]);
  const [me, setMe] = useState("");

  // 筛选条件（客户端过滤，看板已一次性加载全部卡片）；除文本防抖外均直接写 URL。
  const [filterText, setFilterText] = useState(() => get("p_q", ""));
  const filterAssignee = get("p_assignee", "");
  const filterLabel = get("p_label", "");
  const filterState = get("p_state", "");
  useEffect(() => {
    const id = setTimeout(() => setQuery({ p_q: filterText || null }), 300);
    return () => clearTimeout(id);
  }, [filterText, setQuery]);

  const load = useCallback(async () => {
    try {
      setBoard(await api.getBoard(owner, name, project.id));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, project.id, to]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    let alive = true;
    Promise.all([
      api.listLabels(owner, name).catch(() => [] as Label[]),
      api.listCollabs(owner, name).catch(() => [] as { username: string }[]),
      api.listIssues(owner, name, 100, 0).catch(() => ({ items: [] as Issue[], total: 0 })),
      api.me().catch(() => null),
    ]).then(([ls, cs, is, u]) => {
      if (!alive) return;
      setLabels(ls);
      setPeople(Array.from(new Set([owner, ...cs.map((c) => c.username)])));
      setIssues(is.items);
      setMe(u?.username ?? "");
    });
    return () => {
      alive = false;
    };
  }, [owner, name]);

  const columns = useMemo(
    () => (board?.columns ?? []).slice().sort((a, b) => a.position - b.position),
    [board],
  );
  const lanes = useMemo(
    () => (board?.swimlanes ?? []).slice().sort((a, b) => a.position - b.position),
    [board],
  );
  const cards = useMemo(() => board?.cards ?? [], [board]);
  const hasUngrouped = useMemo(() => cards.some((c) => c.swimlane_id === UNGROUPED), [cards]);
  // 客户端筛选：文本 / 负责人 / 标签 / issue 状态。
  const visibleCards = useMemo(() => {
    const q = filterText.trim().toLowerCase();
    return cards.filter((c) => {
      if (filterState && c.issue_state !== filterState) return false;
      const asg = c.assignees ?? [];
      if (filterAssignee === "none" && asg.length > 0) return false;
      if (filterAssignee && filterAssignee !== "none") {
        const want = filterAssignee === "me" ? me : filterAssignee;
        if (!want || !asg.includes(want)) return false;
      }
      if (filterLabel && !(c.labels ?? []).some((l) => String(l.id) === filterLabel)) return false;
      if (q) {
        const hay = `${c.title ?? ""} ${c.body ?? ""} ${c.issue_number ?? ""} ${c.issue_title ?? ""}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  }, [cards, filterText, filterAssignee, filterLabel, filterState, me]);
  // 预分桶：一次性按 (泳道, 列) 归组并排序，避免每次渲染做 L×C 次全量 filter。
  const cardsByCell = useMemo(() => {
    const grouped = new Map<string, ProjectCard[]>();
    for (const c of visibleCards) {
      const key = `${c.swimlane_id}:${c.column_id}`;
      const bucket = grouped.get(key);
      if (bucket) bucket.push(c);
      else grouped.set(key, [c]);
    }
    for (const bucket of grouped.values()) {
      bucket.sort((a, b) => a.position - b.position || a.id - b.id);
    }
    return grouped;
  }, [visibleCards]);

  const act = async (fn: () => Promise<unknown>, msg?: string) => {
    setBusy(true);
    try {
      await fn();
      if (msg) toast.success(t(msg));
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const saveProject = async () => {
    if (!nameInput.trim()) return;
    setBusy(true);
    try {
      const p = await api.updateProject(owner, name, project.id, {
        name: nameInput.trim(),
        description: descInput.trim(),
      });
      toast.success(t("projects.saved"));
      onProjectChanged(p);
      setEditProjectOpen(false);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  // 新建卡片：由对话框提交名称 + Markdown 详情（或关联 issue）。
  // 目标列/泳道由对话框选择（看板单元格会预选该格，列表 / 甘特视图用默认首列首泳道）。
  const createCard = async (draft: CardDraft) => {
    if (!draft.column_id) return;
    if (!draft.issue_number && !draft.title.trim()) return;
    setBusy(true);
    try {
      const card = await api.createCard(owner, name, project.id, {
        column_id: draft.column_id,
        swimlane_id: draft.swimlane_id,
        issue_number: draft.issue_number || undefined,
        title: draft.title.trim() || undefined,
        body: draft.body || undefined,
        start_date: draft.start_date,
        due_date: draft.due_date,
      });
      if (draft.assignees.length > 0) {
        await api.setCardAssignees(owner, name, project.id, card.id, draft.assignees);
      }
      if (draft.label_ids.length > 0) {
        await api.setCardLabels(owner, name, project.id, card.id, draft.label_ids);
      }
      toast.success(t("projects.cardAdded"));
      setAddTarget(null);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const removeCard = async (card: ProjectCard) => {
    await act(() => api.deleteCard(owner, name, project.id, card.id));
  };

  const saveCard = async (card: ProjectCard, draft: CardDraft) => {
    setBusy(true);
    try {
      await api.updateCard(owner, name, project.id, card.id, {
        issue_number: draft.issue_number,
        title: draft.issue_number ? undefined : draft.title.trim(),
        body: draft.issue_number ? undefined : draft.body,
        start_date: draft.start_date,
        due_date: draft.due_date,
      });
      await api.setCardAssignees(owner, name, project.id, card.id, draft.assignees);
      await api.setCardLabels(owner, name, project.id, card.id, draft.label_ids);
      toast.success(t("projects.cardSaved"));
      setEditCard(null);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const createColumn = async () => {
    if (!newColumn.trim()) return;
    await act(() => api.createColumn(owner, name, project.id, newColumn.trim()), "projects.columnAdded");
    setNewColumn("");
    setColumnDialogOpen(false);
  };

  const createSwimlane = async () => {
    if (!newLane.trim()) return;
    await act(() => api.createSwimlane(owner, name, project.id, newLane.trim()), "projects.swimlaneAdded");
    setNewLane("");
    setLaneDialogOpen(false);
  };

  const moveCard = (cardId: number, swimlaneId: number, columnId: number, position = 0) => {
    void act(
      () => api.updateCard(owner, name, project.id, cardId, { column_id: columnId, swimlane_id: swimlaneId, position }),
    );
  };

  const renameColumn = (cid: number, colName: string) =>
    void act(() => api.updateColumn(owner, name, project.id, cid, { name: colName }), "projects.columnRenamed");

  const renameLane = (lid: number, laneName: string) =>
    void act(() => api.updateSwimlane(owner, name, project.id, lid, { name: laneName }), "projects.swimlaneRenamed");

  // 交换相邻列 / 泳道并重新编号 position（列数很小，逐条 PATCH 足够）。
  const reorder = (items: { id: number }[], id: number, dir: -1 | 1, patch: (itemId: number, position: number) => Promise<unknown>) => {
    const i = items.findIndex((x) => x.id === id);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= items.length) return;
    const next = items.slice();
    [next[i], next[j]] = [next[j], next[i]];
    void act(async () => {
      for (let k = 0; k < next.length; k++) await patch(next[k].id, k);
    });
  };

  const moveColumn = (cid: number, dir: -1 | 1) =>
    reorder(columns, cid, dir, (itemId, position) =>
      api.updateColumn(owner, name, project.id, itemId, { position }),
    );

  const moveLane = (lid: number, dir: -1 | 1) =>
    reorder(lanes, lid, dir, (itemId, position) =>
      api.updateSwimlane(owner, name, project.id, itemId, { position }),
    );

  const askDeleteColumn = (cid: number) => {
    const others = columns.filter((c) => c.id !== cid);
    setDeleteColumnMoveTo(others[0]?.id ?? 0);
    setPendingDeleteColumn({ id: cid, count: cards.filter((c) => c.column_id === cid).length });
  };

  const confirmDeleteColumn = () => {
    const p = pendingDeleteColumn;
    setPendingDeleteColumn(null);
    if (!p) return;
    const target = p.count > 0 ? deleteColumnMoveTo : 0;
    void act(
      () => api.deleteColumn(owner, name, project.id, p.id, target || undefined),
      "projects.columnDeleted",
    );
  };

  if (!board) {
    return (
      <div className="space-y-3">
        <Button size="sm" variant="ghost" className="gap-1.5" onClick={onBack}>
          <ArrowLeft className="h-4 w-4" />
          {t("projects.backToList")}
        </Button>
        <p className="rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">
          <Layers className="mr-1.5 inline h-4 w-4" />
          {t("projects.loadingBoard")}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="ghost" className="gap-1.5" onClick={onBack}>
          <ArrowLeft className="h-4 w-4" />
          {t("projects.backToList")}
        </Button>
        <h2 className="text-lg font-semibold">{board.project.name}</h2>
        <Button
          size="sm"
          variant="ghost"
          className="h-8 w-8"
          onClick={() => {
            setNameInput(board.project.name);
            setDescInput(board.project.description);
            setEditProjectOpen(true);
          }}
          title={t("projects.rename")}
        >
          <Pencil className="h-4 w-4" />
        </Button>
        <div className="ml-auto flex items-center gap-1 rounded-md border p-0.5">
          {([
            { id: "board", icon: KanbanSquare, label: t("projects.viewBoard") },
            { id: "list", icon: List, label: t("projects.viewList") },
            { id: "gantt", icon: GanttChartSquare, label: t("projects.viewGantt") },
          ] as const).map((v) => (
            <Button
              key={v.id}
              size="sm"
              variant={view === v.id ? "secondary" : "ghost"}
              className="h-7 gap-1.5 px-2 text-xs"
              onClick={() => setView(v.id)}
              title={v.label}
            >
              <v.icon className="h-3.5 w-3.5" />
              {v.label}
            </Button>
          ))}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-8"
            placeholder={t("projects.filterText")}
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
          />
        </div>
        <select
          aria-label={t("projects.filterAssignee")}
          title={t("projects.filterAssignee")}
          className="h-9 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          value={filterAssignee}
          onChange={(e) => setQuery({ p_assignee: e.target.value || null })}
        >
          <option value="">{t("projects.filterAnyone")}</option>
          <option value="me">{t("projects.filterMine")}</option>
          <option value="none">{t("issues.assigneeNone")}</option>
        </select>
        <select
          aria-label={t("projects.filterLabel")}
          title={t("projects.filterLabel")}
          className="h-9 max-w-44 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          value={filterLabel}
          onChange={(e) => setQuery({ p_label: e.target.value || null })}
        >
          <option value="">{t("projects.filterAllLabels")}</option>
          {labels.map((l) => (
            <option key={l.id} value={String(l.id)}>
              {l.name}
            </option>
          ))}
        </select>
        <select
          aria-label={t("projects.filterState")}
          title={t("projects.filterState")}
          className="h-9 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          value={filterState}
          onChange={(e) => setQuery({ p_state: e.target.value || null })}
        >
          <option value="">{t("projects.filterAnyState")}</option>
          <option value="open">{t("issues.open")}</option>
          <option value="closed">{t("issues.closed")}</option>
        </select>
      </div>

      {view === "list" && (
        <ProjectListView
          columns={columns}
          swimlanes={lanes}
          cards={visibleCards}
          canWrite={canWrite}
          busy={busy}
          onEdit={setEditCard}
          onDelete={(c) => void removeCard(c)}
        />
      )}

      {view === "gantt" && (
        <ProjectGanttView
          columns={columns}
          swimlanes={lanes}
          cards={visibleCards}
          canWrite={canWrite}
          busy={busy}
          onEdit={setEditCard}
          onDelete={(c) => void removeCard(c)}
        />
      )}

      {view === "board" && (
        <ProjectBoardView
          columns={columns}
          lanes={lanes}
          cardsByCell={cardsByCell}
          hasUngrouped={hasUngrouped}
          busy={busy}
          onEditCard={setEditCard}
          onDeleteCard={(c) => void removeCard(c)}
          onDeleteColumn={askDeleteColumn}
          onDeleteLane={setPendingDeleteLane}
          onAddCard={(swimlaneId, columnId) => setAddTarget({ swimlaneId, columnId })}
          onMoveCard={moveCard}
          onRenameColumn={renameColumn}
          onMoveColumn={moveColumn}
          onRenameLane={renameLane}
          onMoveLane={moveLane}
        />
      )}

      {canWrite && (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            className="gap-1.5"
            disabled={busy || columns.length === 0}
            onClick={() =>
              setAddTarget({ columnId: columns[0]?.id ?? 0, swimlaneId: lanes[0]?.id ?? UNGROUPED })
            }
          >
            <Plus className="h-4 w-4" />
            {t("projects.addCard")}
          </Button>
          <Button size="sm" variant="outline" className="gap-1.5" onClick={() => { setNewColumn(""); setColumnDialogOpen(true); }}>
            <SquarePlus className="h-4 w-4" />
            {t("projects.addColumn")}
          </Button>
          <Button size="sm" variant="outline" className="gap-1.5" onClick={() => { setNewLane(""); setLaneDialogOpen(true); }}>
            <SquarePlus className="h-4 w-4" />
            {t("projects.addSwimlane")}
          </Button>
        </div>
      )}

      <Dialog open={editProjectOpen} onOpenChange={setEditProjectOpen}>
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("projects.rename")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <label className="text-xs font-medium">{t("projects.nameLabel")}</label>
              <Input
                autoFocus
                maxLength={100}
                value={nameInput}
                onChange={(e) => setNameInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && void saveProject()}
              />
            </div>
            <div className="grid gap-1.5">
              <label className="text-xs font-medium">{t("projects.descriptionLabel")}</label>
              <Textarea rows={3} value={descInput} onChange={(e) => setDescInput(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setEditProjectOpen(false)} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button onClick={() => void saveProject()} disabled={busy || !nameInput.trim()}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={columnDialogOpen} onOpenChange={setColumnDialogOpen}>
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("projects.addColumn")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-1.5">
            <label className="text-xs text-muted-foreground">{t("projects.columnName")}</label>
            <Input
              autoFocus
              maxLength={50}
              placeholder={t("projects.columnPlaceholder")}
              value={newColumn}
              onChange={(e) => setNewColumn(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && void createColumn()}
            />
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setColumnDialogOpen(false)} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button onClick={() => void createColumn()} disabled={busy || !newColumn.trim()}>
              <SquarePlus className="h-4 w-4" />
              {t("projects.addColumn")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={laneDialogOpen} onOpenChange={setLaneDialogOpen}>
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("projects.addSwimlane")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-1.5">
            <label className="text-xs text-muted-foreground">{t("projects.swimlaneName")}</label>
            <Input
              autoFocus
              maxLength={50}
              placeholder={t("projects.swimlanePlaceholder")}
              value={newLane}
              onChange={(e) => setNewLane(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && void createSwimlane()}
            />
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setLaneDialogOpen(false)} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button onClick={() => void createSwimlane()} disabled={busy || !newLane.trim()}>
              <SquarePlus className="h-4 w-4" />
              {t("projects.addSwimlane")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ProjectCardDialog
        open={editCard !== null}
        mode="edit"
        card={editCard}
        columns={columns}
        swimlanes={lanes}
        people={people}
        labels={labels}
        issues={issues}
        busy={busy}
        onClose={() => setEditCard(null)}
        onSubmit={(d) => editCard && void saveCard(editCard, d)}
      />

      <ProjectCardDialog
        open={addTarget !== null}
        mode="create"
        columns={columns}
        swimlanes={lanes}
        people={people}
        labels={labels}
        issues={issues}
        defaultTarget={addTarget}
        busy={busy}
        onClose={() => setAddTarget(null)}
        onSubmit={(d) => void createCard(d)}
      />

      <Dialog
        open={pendingDeleteColumn !== null}
        onOpenChange={(o) => !o && setPendingDeleteColumn(null)}
      >
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("projects.deleteColumn")}</DialogTitle>
          </DialogHeader>
          {pendingDeleteColumn && pendingDeleteColumn.count > 0 ? (
            <div className="grid gap-3">
              <p className="text-sm text-muted-foreground">
                {t("projects.deleteColumnWithCards", { count: pendingDeleteColumn.count })}
              </p>
              <div className="grid gap-1.5">
                <label className="text-xs font-medium">{t("projects.moveCardsTo")}</label>
                <select
                  className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={deleteColumnMoveTo}
                  onChange={(e) => setDeleteColumnMoveTo(Number(e.target.value))}
                >
                  {columns
                    .filter((c) => c.id !== pendingDeleteColumn.id)
                    .map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.name}
                      </option>
                    ))}
                </select>
              </div>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t("projects.confirmDeleteColumn")}</p>
          )}
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPendingDeleteColumn(null)} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={confirmDeleteColumn}
              disabled={busy}
            >
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={pendingDeleteLane !== null}
        onOpenChange={(o) => !o && setPendingDeleteLane(null)}
        description={t("projects.confirmDeleteSwimlane")}
        onConfirm={() => {
          const lid = pendingDeleteLane;
          setPendingDeleteLane(null);
          if (lid != null) void act(() => api.deleteSwimlane(owner, name, project.id, lid), "projects.swimlaneDeleted");
        }}
        busy={busy}
      />
    </div>
  );
}
