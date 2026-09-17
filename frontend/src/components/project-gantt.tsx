import { useMemo } from "react";
import { CalendarDays, Pencil, Trash2 } from "lucide-react";
import type { ProjectCard } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import {
  BAR_COLORS,
  UNGROUPED,
  addDays,
  cardLabel,
  cardRange,
  dayDiff,
  parseDay,
  fmtDay,
  today,
  type ProjectViewProps,
} from "@/components/project-view-shared";

export function ProjectGanttView({ columns, swimlanes, cards, canWrite, busy, onEdit, onDelete }: ProjectViewProps) {
  const { t, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : lang;

  const scheduled = cards.filter((c) => c.start_date || c.due_date);
  const unscheduled = cards.filter((c) => !c.start_date && !c.due_date);

  const colIndex = (id: number) => Math.max(0, columns.findIndex((c) => c.id === id));
  const laneName = (id: number) => (id === UNGROUPED ? t("projects.ungrouped") : swimlanes.find((l) => l.id === id)?.name ?? "");

  const { min, total } = useMemo(() => {
    let lo: Date | null = null;
    let hi: Date | null = null;
    for (const c of scheduled) {
      const r = cardRange(c);
      if (!r) continue;
      if (!lo || r.start < lo) lo = r.start;
      if (!hi || r.end > hi) hi = r.end;
    }
    if (!lo || !hi) return { min: new Date(), max: new Date(), total: 1 };
    lo = addDays(lo, -1);
    hi = addDays(hi, 1);
    return { min: lo, max: hi, total: Math.max(1, dayDiff(lo, hi) + 1) };
  }, [scheduled]);

  if (scheduled.length === 0) {
    return (
      <div className="space-y-3">
        <p className="rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">{t("projects.ganttEmpty")}</p>
        <UnscheduledList cards={unscheduled} canWrite={canWrite} busy={busy} onEdit={onEdit} onDelete={onDelete} />
      </div>
    );
  }

  const step = Math.max(1, Math.ceil(total / 12));
  const ticks: Date[] = [];
  for (let d = 0; d < total; d += step) ticks.push(addDays(min, d));

  const laneIds = [
    ...swimlanes.map((l) => l.id),
    ...(scheduled.some((c) => c.swimlane_id === UNGROUPED) ? [UNGROUPED] : []),
  ];
  const now = today();
  const todayPct = dayDiff(min, now) >= 0 && dayDiff(min, now) <= total ? (dayDiff(min, now) / total) * 100 : null;

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-lg border">
        <div className="min-w-[48rem]">
          {/* 时间轴表头 */}
          <div className="relative flex border-b bg-muted/40">
            <div className="w-48 shrink-0 border-r px-3 py-2 text-xs font-medium text-muted-foreground">{t("projects.colCard")}</div>
            <div className="relative h-9 flex-1">
              {ticks.map((d) => (
                <span
                  key={d.getTime()}
                  className="absolute top-2 -translate-x-1/2 whitespace-nowrap text-[10px] text-muted-foreground"
                  style={{ left: `${(dayDiff(min, d) / total) * 100}%` }}
                >
                  {fmtDay(d, locale)}
                </span>
              ))}
            </div>
          </div>

          {laneIds.map((laneId) => {
            const laneCards = scheduled.filter((c) => c.swimlane_id === laneId);
            if (laneCards.length === 0) return null;
            return (
              <div key={laneId}>
                <div className="border-b bg-muted/20 px-3 py-1 text-xs font-medium">{laneName(laneId)}</div>
                {laneCards.map((card) => {
                  const r = cardRange(card)!;
                  const left = (dayDiff(min, r.start) / total) * 100;
                  const width = Math.min(100 - left, Math.max(1.5, ((dayDiff(r.start, r.end) + 1) / total) * 100));
                  const color = BAR_COLORS[colIndex(card.column_id) % BAR_COLORS.length];
                  const due = parseDay(card.due_date);
                  const isOverdue = due != null && due < now;
                  return (
                    <div key={card.id} className="group flex items-center border-b last:border-b-0">
                      <div className="flex w-48 shrink-0 items-center gap-1 border-r px-3 py-1.5">
                        <span className="truncate text-xs" title={cardLabel(card)}>
                          {card.issue_number ? `#${card.issue_number} ` : ""}
                          {cardLabel(card)}
                        </span>
                        {canWrite && (
                          <span className="ml-auto flex shrink-0 opacity-0 group-hover:opacity-100">
                            <button className="p-0.5 text-muted-foreground hover:text-foreground" disabled={busy} onClick={() => onEdit(card)} title={t("projects.editCard")}>
                              <Pencil className="h-3 w-3" />
                            </button>
                            <button className="p-0.5 text-destructive" disabled={busy} onClick={() => onDelete(card)} title={t("projects.deleteCard")}>
                              <Trash2 className="h-3 w-3" />
                            </button>
                          </span>
                        )}
                      </div>
                      <div className="relative h-8 flex-1">
                        {todayPct != null && <div className="absolute inset-y-0 w-px bg-destructive/50" style={{ left: `${todayPct}%` }} />}
                        <button
                          className={cn(
                            "absolute top-1.5 h-5 rounded px-1.5 text-left text-[10px] leading-5 text-white shadow-sm",
                            color,
                            isOverdue && "ring-1 ring-destructive",
                          )}
                          style={{ left: `${left}%`, width: `${width}%` }}
                          disabled={!canWrite || busy}
                          onClick={() => onEdit(card)}
                          title={`${cardLabel(card)} · ${card.start_date || "?"} → ${card.due_date || "?"}`}
                        >
                          <span className="block truncate">{cardLabel(card)}</span>
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>

      <UnscheduledList cards={unscheduled} canWrite={canWrite} busy={busy} onEdit={onEdit} onDelete={onDelete} />
    </div>
  );
}

export function UnscheduledList({
  cards,
  canWrite,
  busy,
  onEdit,
  onDelete,
}: {
  cards: ProjectCard[];
  canWrite: boolean;
  busy: boolean;
  onEdit: (card: ProjectCard) => void;
  onDelete: (card: ProjectCard) => void;
}) {
  const { t } = useI18n();
  if (cards.length === 0) return null;
  return (
    <div className="rounded-lg border bg-muted/20 p-3">
      <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <CalendarDays className="h-3.5 w-3.5" />
        {t("projects.unscheduled")}
      </p>
      <div className="flex flex-wrap gap-1.5">
        {cards.map((card) => (
          <span key={card.id} className="group inline-flex items-center gap-1 rounded-full border bg-background px-2 py-0.5 text-xs">
            {card.issue_number ? `#${card.issue_number} ` : ""}
            {cardLabel(card)}
            {canWrite && (
              <span className="flex items-center gap-0.5">
                <button className="text-muted-foreground hover:text-foreground" disabled={busy} onClick={() => onEdit(card)}>
                  <Pencil className="h-3 w-3" />
                </button>
                <button className="text-destructive" disabled={busy} onClick={() => onDelete(card)}>
                  <Trash2 className="h-3 w-3" />
                </button>
              </span>
            )}
          </span>
        ))}
      </div>
    </div>
  );
}

// ---- 卡片编辑对话框（文本 + 日程）----

