import { useEffect, useMemo, useState } from "react";
import { CalendarDays, Pencil, Trash2 } from "lucide-react";
import type { ProjectCard, ProjectColumn, ProjectSwimlane } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

export interface ProjectViewProps {
  columns: ProjectColumn[];
  swimlanes: ProjectSwimlane[];
  cards: ProjectCard[];
  canWrite: boolean;
  busy: boolean;
  onEdit: (card: ProjectCard) => void;
  onDelete: (card: ProjectCard) => void;
}

const UNGROUPED = 0;

// ---- 日期工具（本地时区，避免 UTC 偏移）----

const DAY_MS = 86_400_000;

function parseDay(s: string): Date | null {
  if (!s) return null;
  const [y, m, d] = s.split("-").map(Number);
  if (!y || !m || !d) return null;
  return new Date(y, m - 1, d);
}

function addDays(d: Date, n: number): Date {
  const x = new Date(d);
  x.setDate(x.getDate() + n);
  return x;
}

function dayDiff(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / DAY_MS);
}

function today(): Date {
  const n = new Date();
  return new Date(n.getFullYear(), n.getMonth(), n.getDate());
}

function fmtDay(d: Date, locale: string): string {
  return d.toLocaleDateString(locale, { month: "short", day: "numeric" });
}

/** 卡片的起止（未排期返回 null）。只有一端时按 1 天处理。 */
function cardRange(card: ProjectCard): { start: Date; end: Date } | null {
  const s = parseDay(card.start_date);
  const e = parseDay(card.due_date);
  if (!s && !e) return null;
  return { start: s ?? e!, end: e ?? s! };
}

const BAR_COLORS = [
  "bg-blue-500/80",
  "bg-emerald-500/80",
  "bg-amber-500/80",
  "bg-violet-500/80",
  "bg-rose-500/80",
  "bg-cyan-500/80",
  "bg-orange-500/80",
  "bg-teal-500/80",
];

function cardLabel(card: ProjectCard): string {
  if (card.issue_number) return card.issue_title || `#${card.issue_number}`;
  return card.note || "";
}

// ---- 列表视图 ----

export function ProjectListView({ columns, swimlanes, cards, canWrite, busy, onEdit, onDelete }: ProjectViewProps) {
  const { t, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : lang;
  const colName = (id: number) => columns.find((c) => c.id === id)?.name ?? "";
  const laneName = (id: number) => (id === UNGROUPED ? t("projects.ungrouped") : swimlanes.find((l) => l.id === id)?.name ?? "");
  const overdue = today();

  const sorted = useMemo(
    () =>
      cards.slice().sort((a, b) => {
        const ka = a.due_date || "9999-99-99";
        const kb = b.due_date || "9999-99-99";
        return ka.localeCompare(kb) || a.position - b.position || a.id - b.id;
      }),
    [cards],
  );

  if (sorted.length === 0) {
    return <p className="rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">{t("projects.listEmpty")}</p>;
  }

  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full min-w-[42rem] text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2 font-medium">{t("projects.colCard")}</th>
            <th className="px-3 py-2 font-medium">{t("projects.colColumn")}</th>
            <th className="px-3 py-2 font-medium">{t("projects.colSwimlane")}</th>
            <th className="px-3 py-2 font-medium">{t("projects.colStart")}</th>
            <th className="px-3 py-2 font-medium">{t("projects.colDue")}</th>
            {canWrite && <th className="px-3 py-2" />}
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {sorted.map((card) => {
            const due = parseDay(card.due_date);
            const isOverdue = due != null && due < overdue;
            return (
              <tr key={card.id} className="hover:bg-muted/30">
                <td className="max-w-xs px-3 py-2">
                  <div className="flex items-center gap-2">
                    {card.issue_number ? (
                      <>
                        <span className="shrink-0 font-mono text-xs text-muted-foreground">#{card.issue_number}</span>
                        <span className="truncate">{card.issue_title || `#${card.issue_number}`}</span>
                        {card.issue_state && (
                          <Badge
                            variant="outline"
                            className={cn(
                              "h-4 shrink-0 px-1 text-[10px]",
                              card.issue_state === "open" ? "border-green-600 text-green-600" : "border-purple-600 text-purple-600",
                            )}
                          >
                            {card.issue_state}
                          </Badge>
                        )}
                      </>
                    ) : (
                      <span className="whitespace-pre-wrap break-words">{card.note}</span>
                    )}
                  </div>
                </td>
                <td className="whitespace-nowrap px-3 py-2 text-muted-foreground">{colName(card.column_id)}</td>
                <td className="whitespace-nowrap px-3 py-2 text-muted-foreground">{laneName(card.swimlane_id)}</td>
                <td className="whitespace-nowrap px-3 py-2 text-muted-foreground">{card.start_date || "—"}</td>
                <td className={cn("whitespace-nowrap px-3 py-2", isOverdue ? "font-medium text-destructive" : "text-muted-foreground")}>
                  {card.due_date || "—"}
                </td>
                {canWrite && (
                  <td className="whitespace-nowrap px-3 py-2 text-right">
                    <Button variant="ghost" size="icon" className="h-7 w-7" disabled={busy} onClick={() => onEdit(card)} title={t("projects.editCard")}>
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-destructive hover:text-destructive"
                      disabled={busy}
                      onClick={() => onDelete(card)}
                      title={t("projects.deleteCard")}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </td>
                )}
              </tr>
            );
          })}
        </tbody>
      </table>
      <p className="border-t px-3 py-1.5 text-xs text-muted-foreground">{locale && t("projects.listCount", { count: sorted.length })}</p>
    </div>
  );
}

// ---- 甘特视图 ----

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

function UnscheduledList({
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

export interface CardDraft {
  note: string;
  start_date: string;
  due_date: string;
}

export function ProjectCardDialog({
  card,
  onClose,
  onSave,
  busy,
}: {
  card: ProjectCard | null;
  onClose: () => void;
  onSave: (card: ProjectCard, draft: CardDraft) => void;
  busy: boolean;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState<CardDraft>({ note: "", start_date: "", due_date: "" });

  // 打开卡片时重置表单
  useEffect(() => {
    if (card) {
      setDraft({ note: card.note ?? "", start_date: card.start_date ?? "", due_date: card.due_date ?? "" });
    }
  }, [card]);

  const invalidRange = draft.start_date !== "" && draft.due_date !== "" && draft.due_date < draft.start_date;

  return (
    <Dialog open={card !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t("projects.editCard")}
            {card?.issue_number ? ` · #${card.issue_number}` : ""}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          {!card?.issue_number && (
            <div className="grid gap-1.5">
              <Label htmlFor="card-note">{t("projects.note")}</Label>
              <Textarea id="card-note" rows={3} value={draft.note} onChange={(e) => setDraft({ ...draft, note: e.target.value })} />
            </div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="card-start">{t("projects.startDate")}</Label>
              <Input id="card-start" type="date" value={draft.start_date} onChange={(e) => setDraft({ ...draft, start_date: e.target.value })} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="card-due">{t("projects.dueDate")}</Label>
              <Input id="card-due" type="date" value={draft.due_date} onChange={(e) => setDraft({ ...draft, due_date: e.target.value })} />
            </div>
          </div>
          {invalidRange && <p className="text-xs text-destructive">{t("projects.invalidRange")}</p>}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button onClick={() => card && onSave(card, draft)} disabled={busy || invalidRange}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
