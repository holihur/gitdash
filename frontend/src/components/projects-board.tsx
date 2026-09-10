import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { ArrowLeft, Layers, Pencil, Plus, SquarePlus, Trash2, X } from "lucide-react";
import { api, type Project, type ProjectBoard } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import ConfirmDialog from "@/components/confirm-dialog";
import { cn } from "@/lib/utils";

interface Props {
  owner: string;
  name: string;
  project: Project;
  onBack: () => void;
  onProjectChanged: (p: Project) => void;
}

const UNGROUPED = 0;

export default function ProjectsBoard({ owner, name, project, onBack, onProjectChanged }: Props) {
  const { t, to } = useI18n();
  const [board, setBoard] = useState<ProjectBoard | null>(null);
  const [busy, setBusy] = useState(false);

  const [editingName, setEditingName] = useState(false);
  const [nameInput, setNameInput] = useState(project.name);

  const [newColumn, setNewColumn] = useState("");
  const [newLane, setNewLane] = useState("");
  const [pendingDeleteColumn, setPendingDeleteColumn] = useState<number | null>(null);
  const [pendingDeleteLane, setPendingDeleteLane] = useState<number | null>(null);

  // "+ 添加卡片" 行内输入状态：`${swimlaneId}:${columnId}` → 输入内容
  const [cardInputFor, setCardInputFor] = useState<string | null>(null);
  const [cardText, setCardText] = useState("");
  const [dragging, setDragging] = useState<number | null>(null);

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

  const columns = (board?.columns ?? []).slice().sort((a, b) => a.position - b.position);
  const lanes = (board?.swimlanes ?? []).slice().sort((a, b) => a.position - b.position);
  const cards = board?.cards ?? [];
  const hasUngrouped = cards.some((c) => c.swimlane_id === UNGROUPED);

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

  const addCard = async (swimlaneId: number, columnId: number) => {
    const raw = cardText.trim();
    setCardInputFor(null);
    setCardText("");
    if (!raw) return;
    const m = raw.match(/^#(\d+)$/);
    if (m) {
      await act(() => api.createCard(owner, name, project.id, { column_id: columnId, swimlane_id: swimlaneId, issue_number: Number(m[1]) }), "projects.cardAdded");
    } else {
      await act(() => api.createCard(owner, name, project.id, { column_id: columnId, swimlane_id: swimlaneId, note: raw }), "projects.cardAdded");
    }
  };

  const onDrop = (swimlaneId: number, columnId: number) => {
    const cardId = dragging;
    setDragging(null);
    if (cardId == null) return;
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

  const laneRows: { id: number; title: string }[] = [
    ...lanes.map((l) => ({ id: l.id, title: l.name })),
    ...(hasUngrouped ? [{ id: UNGROUPED, title: t("projects.ungrouped") }] : []),
  ];

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
      </div>

      <div className="space-y-3 overflow-x-auto">
        {laneRows.map((lane) => (
          <div key={lane.id} className="min-w-max rounded-lg border bg-muted/20">
            <div className="flex items-center justify-between gap-2 px-3 py-2">
              <p className="text-sm font-medium">{lane.title}</p>
              {lane.id !== UNGROUPED && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 text-destructive hover:text-destructive"
                  disabled={busy}
                  onClick={() => setPendingDeleteLane(lane.id)}
                  title={t("projects.deleteSwimlane")}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              )}
            </div>
            <div className="flex gap-3 overflow-x-auto px-3 pb-3">
              {columns.map((col) => {
                const colCards = cards.filter((c) => c.swimlane_id === lane.id && c.column_id === col.id);
                return (
                  <div
                    key={col.id}
                    className={cn(
                      "flex w-64 shrink-0 flex-col rounded-md border bg-card",
                      dragging != null && "border-dashed border-primary/50",
                    )}
                    onDragOver={(e) => e.preventDefault()}
                    onDrop={() => onDrop(lane.id, col.id)}
                  >
                    <div className="flex items-center justify-between gap-1 px-2 py-1.5">
                      <p className="truncate text-xs font-medium text-muted-foreground">
                        {col.name}
                        <span className="ml-1.5">{colCards.length}</span>
                      </p>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 text-destructive hover:text-destructive"
                        disabled={busy}
                        onClick={() => setPendingDeleteColumn(col.id)}
                        title={t("projects.deleteColumn")}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                    <div className="flex-1 space-y-2 px-2 pb-2">
                      {colCards.map((card) => (
                        <div
                          key={card.id}
                          draggable
                          onDragStart={() => setDragging(card.id)}
                          onDragEnd={() => setDragging(null)}
                          className="group cursor-grab space-y-1 rounded-md border bg-background p-2 text-sm active:cursor-grabbing"
                        >
                          <div className="flex items-start justify-between gap-1">
                            <div className="min-w-0 flex-1">
                              {card.issue_number != null ? (
                                <>
                                  <p className="truncate">{card.issue_title || `#${card.issue_number}`}</p>
                                  <p className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                                    #{card.issue_number}
                                    <Badge
                                      variant="outline"
                                      className={cn(
                                        "h-4 px-1 text-[10px]",
                                        card.issue_state === "open"
                                          ? "border-green-600 text-green-600"
                                          : "border-purple-600 text-purple-600",
                                      )}
                                    >
                                      {card.issue_state}
                                    </Badge>
                                  </p>
                                </>
                              ) : (
                                <p className="whitespace-pre-wrap break-words">{card.note}</p>
                              )}
                            </div>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-6 w-6 shrink-0 text-destructive opacity-0 hover:text-destructive group-hover:opacity-100"
                              disabled={busy}
                              onClick={() => act(() => api.deleteCard(owner, name, project.id, card.id))}
                              title={t("projects.deleteCard")}
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </div>
                      ))}
                      {cardInputFor === `${lane.id}:${col.id}` ? (
                        <Input
                          autoFocus
                          className="h-8 text-xs"
                          placeholder={t("projects.cardInputHint")}
                          onBlur={() => setCardInputFor(null)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") void addCard(lane.id, col.id);
                            if (e.key === "Escape") setCardInputFor(null);
                          }}
                          onChange={(e) => setCardText(e.target.value)}
                          value={cardText}
                        />
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-full justify-center gap-1 text-xs text-muted-foreground"
                          onClick={() => { setCardText(""); setCardInputFor(`${lane.id}:${col.id}`); }}
                        >
                          <Plus className="h-3.5 w-3.5" />
                          {t("projects.addCard")}
                        </Button>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>

      <div className="flex flex-wrap items-end gap-2 rounded-lg border bg-muted/30 p-3">
        <div className="grid gap-1">
          <label className="text-xs text-muted-foreground">{t("projects.columnName")}</label>
          <Input
            className="h-8 w-40"
            maxLength={50}
            placeholder={t("projects.columnPlaceholder")}
            value={newColumn}
            onChange={(e) => setNewColumn(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && newColumn.trim() && (void act(() => api.createColumn(owner, name, project.id, newColumn.trim()), "projects.columnAdded").then(() => setNewColumn("")))}
          />
        </div>
        <Button size="sm" className="gap-1.5" disabled={busy || !newColumn.trim()} onClick={() => act(() => api.createColumn(owner, name, project.id, newColumn.trim()), "projects.columnAdded").then(() => setNewColumn(""))}>
          <SquarePlus className="h-4 w-4" />
          {t("projects.addColumn")}
        </Button>
        <div className="grid gap-1">
          <label className="text-xs text-muted-foreground">{t("projects.swimlaneName")}</label>
          <Input
            className="h-8 w-40"
            maxLength={50}
            placeholder={t("projects.swimlanePlaceholder")}
            value={newLane}
            onChange={(e) => setNewLane(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && newLane.trim() && (void act(() => api.createSwimlane(owner, name, project.id, newLane.trim()), "projects.swimlaneAdded").then(() => setNewLane("")))}
          />
        </div>
        <Button size="sm" className="gap-1.5" disabled={busy || !newLane.trim()} onClick={() => act(() => api.createSwimlane(owner, name, project.id, newLane.trim()), "projects.swimlaneAdded").then(() => setNewLane(""))}>
          <SquarePlus className="h-4 w-4" />
          {t("projects.addSwimlane")}
        </Button>
      </div>

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
