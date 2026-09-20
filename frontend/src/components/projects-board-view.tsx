import { useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Pencil, Trash2 } from "lucide-react";
import type { ProjectCard, ProjectColumn, ProjectSwimlane } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { MarkdownView } from "@/components/markdown";
import { AddCardCell, VirtualCardList } from "@/components/projects-board-parts";

/** 未分组泳道的哨兵 id */
export const UNGROUPED = 0;

/** 看板（Kanban）视图：泳道纵向虚拟滚动 + 泳道内横向列。 */
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
  onMoveCard: (cardId: number, swimlaneId: number, columnId: number) => void;
}) {
  const { t } = useI18n();
  const [dragging, setDragging] = useState<number | null>(null);

  const laneRows = useMemo(
    () => [
      ...lanes.map((l) => ({ id: l.id, title: l.name })),
      ...(hasUngrouped ? [{ id: UNGROUPED, title: t("projects.ungrouped") }] : []),
    ],
    [lanes, hasUngrouped, t],
  );

  // 泳道纵向虚拟滚动：看板自身作为滚动容器，只挂载视口附近的泳道。
  const boardScrollRef = useRef<HTMLDivElement>(null);
  const laneVirtualizer = useVirtualizer({
    count: laneRows.length,
    getScrollElement: () => boardScrollRef.current,
    estimateSize: () => 280,
    overscan: 1,
  });
  // 泳道内的列是固定宽度（w-64）+ 间距，给虚拟层一个显式最小宽度以保留横向滚动。
  // 2.5rem = 泳道容器 px-2（1rem）+ 列容器 px-3（1.5rem）。
  const boardMinWidth = useMemo(
    () => `${columns.length * 16 + Math.max(0, columns.length - 1) * 0.75 + 2.5}rem`,
    [columns.length],
  );

  return (
    <div ref={boardScrollRef} className="h-[70vh] overflow-auto rounded-lg border bg-muted/10">
      <div className="relative" style={{ height: laneVirtualizer.getTotalSize(), minWidth: boardMinWidth }}>
        {laneVirtualizer.getVirtualItems().map((vi) => {
          const lane = laneRows[vi.index];
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
                  <p className="text-sm font-medium">{lane.title}</p>
                  {lane.id !== UNGROUPED && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-destructive hover:text-destructive"
                      disabled={busy}
                      onClick={() => onDeleteLane(lane.id)}
                      title={t("projects.deleteSwimlane")}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </div>
                <div className="flex gap-3 px-3 pb-3">
                  {columns.map((col) => {
                    const colCards = cardsByCell.get(`${lane.id}:${col.id}`) ?? [];
                    return (
                      <div
                        key={col.id}
                        className={cn(
                          "flex w-64 shrink-0 flex-col rounded-md border bg-card",
                          dragging != null && "border-dashed border-primary/50",
                        )}
                        onDragOver={(e) => e.preventDefault()}
                        onDrop={() => {
                          const cardId = dragging;
                          setDragging(null);
                          if (cardId != null) onMoveCard(cardId, lane.id, col.id);
                        }}
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
                            onClick={() => onDeleteColumn(col.id)}
                            title={t("projects.deleteColumn")}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                        {colCards.length > 0 && (
                          <VirtualCardList
                            cards={colCards}
                            renderCard={(card) => (
                              <BoardCard
                                card={card}
                                busy={busy}
                                onDragStart={() => setDragging(card.id)}
                                onDragEnd={() => setDragging(null)}
                                onEdit={() => onEditCard(card)}
                                onDelete={() => onDeleteCard(card)}
                              />
                            )}
                          />
                        )}
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

/** 看板中的单张卡片（可拖拽、可编辑/删除）。 */
function BoardCard({
  card,
  busy,
  onDragStart,
  onDragEnd,
  onEdit,
  onDelete,
}: {
  card: ProjectCard;
  busy: boolean;
  onDragStart: () => void;
  onDragEnd: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useI18n();
  return (
    <div
      draggable
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      className="group cursor-grab space-y-1 rounded-md border bg-background p-2 text-sm active:cursor-grabbing"
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
        </div>
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
