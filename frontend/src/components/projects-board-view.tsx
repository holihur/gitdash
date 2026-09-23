import { useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  MoreHorizontal,
  Move,
  Pencil,
  Trash2,
} from "lucide-react";
import type { ProjectCard, ProjectColumn, ProjectSwimlane } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { MarkdownView } from "@/components/markdown";
import LabelChip from "@/components/label-chip";
import { AddCardCell, VirtualCardList } from "@/components/projects-board-parts";

/** 未分组泳道的哨兵 id */
export const UNGROUPED = 0;

/** 行内重命名输入框：Enter 提交 / Esc 取消 / 失焦提交（值未变则保持编辑态）。 */
function InlineRename({
  value,
  onCommit,
  onCancel,
  className,
}: {
  value: string;
  onCommit: (v: string) => void;
  onCancel: () => void;
  className?: string;
}) {
  const [text, setText] = useState(value);
  const commit = () => {
    const v = text.trim();
    if (!v) {
      onCancel();
      return;
    }
    if (v === value) return;
    onCommit(v);
  };
  return (
    <Input
      autoFocus
      value={text}
      className={cn("h-6 px-1.5 py-0 text-xs", className)}
      onChange={(e) => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          commit();
        } else if (e.key === "Escape") {
          e.preventDefault();
          onCancel();
        }
      }}
    />
  );
}

/** 看板（Kanban）视图：sticky 列头 + 泳道纵向虚拟滚动。 */
export function ProjectBoardView({
  columns,
  lanes,
  cardsByCell,
  hasUngrouped,
  busy,
  onEditCard,
  onDeleteCard,
  onDeleteColumn,
  onDeleteLane,
  onAddCard,
  onMoveCard,
  onRenameColumn,
  onMoveColumn,
  onRenameLane,
  onMoveLane,
}: {
  columns: ProjectColumn[];
  lanes: ProjectSwimlane[];
  cardsByCell: Map<string, ProjectCard[]>;
  hasUngrouped: boolean;
  busy: boolean;
  onEditCard: (card: ProjectCard) => void;
  onDeleteCard: (card: ProjectCard) => void;
  onDeleteColumn: (id: number) => void;
  onDeleteLane: (id: number) => void;
  onAddCard: (swimlaneId: number, columnId: number) => void;
  onMoveCard: (cardId: number, swimlaneId: number, columnId: number, position?: number) => void;
  onRenameColumn: (id: number, name: string) => void;
  onMoveColumn: (id: number, dir: -1 | 1) => void;
  onRenameLane: (id: number, name: string) => void;
  onMoveLane: (id: number, dir: -1 | 1) => void;
}) {
  const { t } = useI18n();
  const [dragging, setDragging] = useState<number | null>(null);
  const [editingCol, setEditingCol] = useState<number | null>(null);
  const [editingLane, setEditingLane] = useState<number | null>(null);
  const [dropHint, setDropHint] = useState<{ cell: string; index: number } | null>(null);

  const laneRows = useMemo(
    () => [
      ...lanes.map((l) => ({ id: l.id, title: l.name })),
      ...(hasUngrouped ? [{ id: UNGROUPED, title: t("projects.ungrouped") }] : []),
    ],
    [lanes, hasUngrouped, t],
  );

  // 每列总数（跨泳道求和），用于 sticky 列头。
  const columnTotals = useMemo(() => {
    const m = new Map<number, number>();
    for (const [key, list] of cardsByCell) {
      const col = Number(key.split(":")[1]);
      m.set(col, (m.get(col) ?? 0) + list.length);
    }
    return m;
  }, [cardsByCell]);

  // 泳道纵向虚拟滚动：看板自身作为滚动容器，只挂载视口附近的泳道。
  const boardScrollRef = useRef<HTMLDivElement>(null);
  const laneVirtualizer = useVirtualizer({
    count: laneRows.length,
    getScrollElement: () => boardScrollRef.current,
    estimateSize: () => 280,
    overscan: 1,
  });
  // 2.5rem = 泳道容器 px-2（1rem）+ 列容器 px-3（1.5rem）。
  const boardMinWidth = useMemo(
    () => `${columns.length * 16 + Math.max(0, columns.length - 1) * 0.75 + 2.5}rem`,
    [columns.length],
  );

  return (
    <div ref={boardScrollRef} className="h-[70vh] overflow-auto rounded-lg border bg-muted/10">
      {/* sticky 列头：只渲染一次，滚动时保持在顶部 */}
      <div
        className="sticky top-0 z-10 border-b bg-card/95 py-1.5 pl-[1.25rem] pr-2 backdrop-blur"
        style={{ minWidth: boardMinWidth }}
      >
        <div className="flex gap-3">
          {columns.map((col, ci) => (
            <div
              key={col.id}
              className="flex w-64 shrink-0 items-center justify-between gap-1 rounded-md border bg-muted/40 px-2 py-1"
            >
              {editingCol === col.id ? (
                <InlineRename
                  value={col.name}
                  className="max-w-40"
                  onCommit={(v) => {
                    onRenameColumn(col.id, v);
                    setEditingCol(null);
                  }}
                  onCancel={() => setEditingCol(null)}
                />
              ) : (
                <p className="truncate text-xs font-medium text-muted-foreground">
                  {col.name}
                  <span className="ml-1.5">{columnTotals.get(col.id) ?? 0}</span>
                </p>
              )}
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6 text-muted-foreground"
                    disabled={busy}
                    title={t("projects.columnActions")}
                  >
                    <MoreHorizontal className="h-3.5 w-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => setEditingCol(col.id)}>
                    <Pencil className="h-4 w-4" />
                    {t("projects.renameColumn")}
                  </DropdownMenuItem>
                  <DropdownMenuItem disabled={ci === 0} onClick={() => onMoveColumn(col.id, -1)}>
                    <ArrowLeft className="h-4 w-4" />
                    {t("projects.moveLeft")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={ci === columns.length - 1}
                    onClick={() => onMoveColumn(col.id, 1)}
                  >
                    <ArrowRight className="h-4 w-4" />
                    {t("projects.moveRight")}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onClick={() => onDeleteColumn(col.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                    {t("projects.deleteColumn")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ))}
        </div>
      </div>

      <div className="relative" style={{ height: laneVirtualizer.getTotalSize(), minWidth: boardMinWidth }}>
        {laneVirtualizer.getVirtualItems().map((vi) => {
          const lane = laneRows[vi.index];
          const laneIdx = lanes.findIndex((l) => l.id === lane.id);
          return (
            <div
              key={lane.id}
              data-index={vi.index}
              ref={laneVirtualizer.measureElement}
              className="absolute left-0 top-0 w-full px-2 py-1.5"
              style={{ transform: `translateY(${vi.start}px)` }}
            >
              <div className="rounded-lg border bg-muted/20">
                <div className="flex items-center justify-between gap-2 px-3 py-2">
                  {editingLane === lane.id ? (
                    <InlineRename
                      value={lane.title}
                      className="max-w-48 text-sm"
                      onCommit={(v) => {
                        onRenameLane(lane.id, v);
                        setEditingLane(null);
                      }}
                      onCancel={() => setEditingLane(null)}
                    />
                  ) : (
                    <p className="text-sm font-medium">{lane.title}</p>
                  )}
                  {lane.id !== UNGROUPED && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7 text-muted-foreground"
                          disabled={busy}
                          title={t("projects.swimlaneActions")}
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => setEditingLane(lane.id)}>
                          <Pencil className="h-4 w-4" />
                          {t("projects.renameSwimlane")}
                        </DropdownMenuItem>
                        <DropdownMenuItem disabled={laneIdx <= 0} onClick={() => onMoveLane(lane.id, -1)}>
                          <ArrowUp className="h-4 w-4" />
                          {t("projects.moveUp")}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          disabled={laneIdx < 0 || laneIdx >= lanes.length - 1}
                          onClick={() => onMoveLane(lane.id, 1)}
                        >
                          <ArrowDown className="h-4 w-4" />
                          {t("projects.moveDown")}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onClick={() => onDeleteLane(lane.id)}
                        >
                          <Trash2 className="h-4 w-4" />
                          {t("projects.deleteSwimlane")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </div>
                <div className="flex gap-3 px-3 pb-3">
                  {columns.map((col) => {
                    const cellKey = `${lane.id}:${col.id}`;
                    const colCards = cardsByCell.get(cellKey) ?? [];
                    const hint = dropHint?.cell === cellKey ? dropHint.index : null;
                    return (
                      <div
                        key={col.id}
                        className={cn(
                          "flex w-64 shrink-0 flex-col rounded-md border bg-card",
                          dragging != null && "border-dashed border-primary/50",
                        )}
                        onDrop={() => {
                          const cardId = dragging;
                          const index = hint ?? colCards.length;
                          setDragging(null);
                          setDropHint(null);
                          if (cardId != null) onMoveCard(cardId, lane.id, col.id, index);
                        }}
                      >
                        <div
                          className="relative min-h-3 flex-1"
                          onDragOver={(e) => {
                            e.preventDefault();
                            const rect = e.currentTarget.getBoundingClientRect();
                            const y = e.clientY - rect.top;
                            const index = colCards.length === 0 ? 0 : Math.round(y / 72);
                            setDropHint({ cell: cellKey, index: Math.max(0, Math.min(colCards.length, index)) });
                          }}
                          onDragLeave={() => setDropHint((h) => (h?.cell === cellKey ? null : h))}
                        >
                          {colCards.length > 0 && (
                            <VirtualCardList
                              cards={colCards}
                              dropIndex={hint}
                              renderCard={(card, i) => (
                                <BoardCard
                                  card={card}
                                  columns={columns}
                                  lanes={lanes}
                                  busy={busy}
                                  onDragStart={() => setDragging(card.id)}
                                  onDragEnd={() => {
                                    setDragging(null);
                                    setDropHint(null);
                                  }}
                                  onEdit={() => onEditCard(card)}
                                  onDelete={() => onDeleteCard(card)}
                                  onMove={(laneId, colId) => onMoveCard(card.id, laneId, colId, 0)}
                                  trailing={hint === i && dragging != null}
                                />
                              )}
                            />
                          )}
                        </div>
                        <div className="px-2 pb-2 pt-1">
                          <AddCardCell
                            label={t("projects.addCard")}
                            onClick={() => onAddCard(lane.id, col.id)}
                          />
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** 看板中的单张卡片（可拖拽、可编辑/删除、可用菜单移动到其它列 / 泳道）。 */
function BoardCard({
  card,
  columns,
  lanes,
  busy,
  onDragStart,
  onDragEnd,
  onEdit,
  onDelete,
  onMove,
  trailing,
}: {
  card: ProjectCard;
  columns: ProjectColumn[];
  lanes: ProjectSwimlane[];
  busy: boolean;
  onDragStart: () => void;
  onDragEnd: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onMove: (swimlaneId: number, columnId: number) => void;
  trailing?: boolean;
}) {
  const { t } = useI18n();
  return (
    <div
      draggable
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      className={cn(
        "group cursor-grab space-y-1 rounded-md border bg-background p-2 text-sm active:cursor-grabbing",
        trailing && "border-b-2 border-b-primary",
      )}
    >
      <div className="flex items-start justify-between gap-1">
        <div className="min-w-0 flex-1">
          {card.issue_number ? (
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
            <>
              <p className="break-words font-medium">{card.title || card.note}</p>
              {card.body && (
                <div className="mt-1 max-h-24 overflow-hidden text-xs text-muted-foreground">
                  <MarkdownView text={card.body} className="text-xs" />
                </div>
              )}
            </>
          )}
          {((card.labels?.length ?? 0) > 0 || (card.assignees?.length ?? 0) > 0) && (
            <div className="mt-1 flex flex-wrap items-center gap-1">
              {(card.labels ?? []).map((l) => (
                <LabelChip key={l.id} label={l} />
              ))}
              {(card.assignees ?? []).map((u) => (
                <span key={u} className="rounded-full border px-1.5 text-[10px] text-muted-foreground">
                  {u}
                </span>
              ))}
            </div>
          )}
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100"
              disabled={busy}
              title={t("projects.moveCard")}
            >
              <Move className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="max-h-72 overflow-auto">
            <DropdownMenuLabel className="text-xs">{t("projects.moveToColumn")}</DropdownMenuLabel>
            {columns.map((c) => (
              <DropdownMenuItem
                key={`c${c.id}`}
                disabled={c.id === card.column_id}
                onClick={() => onMove(card.swimlane_id, c.id)}
              >
                {c.name}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="text-xs">{t("projects.moveToSwimlane")}</DropdownMenuLabel>
            <DropdownMenuItem
              disabled={card.swimlane_id === UNGROUPED}
              onClick={() => onMove(UNGROUPED, card.column_id)}
            >
              {t("projects.ungrouped")}
            </DropdownMenuItem>
            {lanes.map((l) => (
              <DropdownMenuItem
                key={`l${l.id}`}
                disabled={l.id === card.swimlane_id}
                onClick={() => onMove(l.id, card.column_id)}
              >
                {l.name}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0 opacity-0 group-hover:opacity-100"
          disabled={busy}
          onClick={onEdit}
          title={t("projects.editCard")}
        >
          <Pencil className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0 text-destructive opacity-0 hover:text-destructive group-hover:opacity-100"
          disabled={busy}
          onClick={onDelete}
          title={t("projects.deleteCard")}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}
