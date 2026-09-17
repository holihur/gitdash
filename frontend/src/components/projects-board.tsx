import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ArrowLeft, GanttChartSquare, KanbanSquare, Layers, List, Pencil, SquarePlus, X } from "lucide-react";
import { api, type Project, type ProjectBoard, type ProjectCard } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
  const [board, setBoard] = useState<ProjectBoard | null>(null);
  const [busy, setBusy] = useState(false);
  const [view, setView] = useState<ProjectView>("board");
  const [editCard, setEditCard] = useState<ProjectCard | null>(null);

  const [editingName, setEditingName] = useState(false);
  const [nameInput, setNameInput] = useState(project.name);

  const [newColumn, setNewColumn] = useState("");
  const [newLane, setNewLane] = useState("");
  const [columnDialogOpen, setColumnDialogOpen] = useState(false);
  const [laneDialogOpen, setLaneDialogOpen] = useState(false);
  const [pendingDeleteColumn, setPendingDeleteColumn] = useState<number | null>(null);
  const [pendingDeleteLane, setPendingDeleteLane] = useState<number | null>(null);
  // 新建卡片目标单元格（点击单元格内的“添加卡片”后打开对话框）
  const [addTarget, setAddTarget] = useState<{ swimlaneId: number; columnId: number } | null>(null);

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
  // 预分桶：一次性按 (泳道, 列) 归组并排序，避免每次渲染做 L×C 次全量 filter。
  const cardsByCell = useMemo(() => {
    const grouped = new Map<string, ProjectCard[]>();
    for (const c of cards) {
      const key = `${c.swimlane_id}:${c.column_id}`;
      const bucket = grouped.get(key);
      if (bucket) bucket.push(c);
      else grouped.set(key, [c]);
    }
    for (const bucket of grouped.values()) {
      bucket.sort((a, b) => a.position - b.position || a.id - b.id);
    }
    return grouped;
  }, [cards]);

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

  const renameProject = async () => {
    if (!nameInput.trim()) return;
    setBusy(true);
    try {
      const p = await api.updateProject(owner, name, project.id, { name: nameInput.trim() });
      toast.success(t("projects.saved"));
      onProjectChanged(p);
      setEditingName(false);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  // 新建卡片：由对话框提交名称 + Markdown 详情（或关联 issue）。
  const createCard = async (draft: CardDraft) => {
    if (!addTarget) return;
    if (!draft.issue_number && !draft.title.trim()) return;
    setBusy(true);
    try {
      await api.createCard(owner, name, project.id, {
        column_id: addTarget.columnId,
        swimlane_id: addTarget.swimlaneId,
        issue_number: draft.issue_number || undefined,
        title: draft.title.trim() || undefined,
        body: draft.body || undefined,
        start_date: draft.start_date,
        due_date: draft.due_date,
      });
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
        title: card.issue_number ? undefined : draft.title.trim(),
        body: card.issue_number ? undefined : draft.body,
        start_date: draft.start_date,
        due_date: draft.due_date,
      });
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

  const moveCard = (cardId: number, swimlaneId: number, columnId: number) => {
    const card = cards.find((c) => c.id === cardId);
    if (!card) return;
    void act(
      () => api.updateCard(owner, name, project.id, cardId, { column_id: columnId, swimlane_id: swimlaneId, position: 0 }),
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
        {editingName ? (
          <>
            <Input
              className="h-8 w-48"
              value={nameInput}
              maxLength={100}
              onChange={(e) => setNameInput(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && renameProject()}
            />
            <Button size="sm" disabled={busy || !nameInput.trim()} onClick={renameProject}>
              {t("common.save")}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { setEditingName(false); setNameInput(board.project.name); }}>
              <X className="h-4 w-4" />
            </Button>
          </>
        ) : (
          <>
            <h2 className="text-lg font-semibold">{board.project.name}</h2>
            <Button size="sm" variant="ghost" className="h-8 w-8" onClick={() => { setEditingName(true); setNameInput(board.project.name); }} title={t("projects.rename")}>
              <Pencil className="h-4 w-4" />
            </Button>
          </>
        )}
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

      {view === "list" && (
        <ProjectListView
          columns={columns}
          swimlanes={lanes}
          cards={cards}
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
          cards={cards}
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
          onDeleteColumn={setPendingDeleteColumn}
          onDeleteLane={setPendingDeleteLane}
          onAddCard={(swimlaneId, columnId) => setAddTarget({ swimlaneId, columnId })}
          onMoveCard={moveCard}
        />
      )}

      {canWrite && (
        <div className="flex flex-wrap items-center gap-2">
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
        busy={busy}
        onClose={() => setEditCard(null)}
        onSubmit={(d) => editCard && void saveCard(editCard, d)}
      />

      <ProjectCardDialog
        open={addTarget !== null}
        mode="create"
        busy={busy}
        onClose={() => setAddTarget(null)}
        onSubmit={(d) => void createCard(d)}
      />

      <ConfirmDialog
        open={pendingDeleteColumn !== null}
        onOpenChange={(o) => !o && setPendingDeleteColumn(null)}
        description={t("projects.confirmDeleteColumn")}
        onConfirm={() => {
          const cid = pendingDeleteColumn;
          setPendingDeleteColumn(null);
          if (cid != null) void act(() => api.deleteColumn(owner, name, project.id, cid), "projects.columnDeleted");
        }}
        busy={busy}
      />
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
