import type { ProjectCard, ProjectColumn, ProjectSwimlane } from "@/lib/api";

export interface ProjectViewProps {
  columns: ProjectColumn[];
  swimlanes: ProjectSwimlane[];
  cards: ProjectCard[];
  canWrite: boolean;
  busy: boolean;
  onEdit: (card: ProjectCard) => void;
  onDelete: (card: ProjectCard) => void;
}

export const UNGROUPED = 0;

// ---- 日期工具（本地时区，避免 UTC 偏移）----

export const DAY_MS = 86_400_000;

export function parseDay(s: string): Date | null {
  if (!s) return null;
  const [y, m, d] = s.split("-").map(Number);
  if (!y || !m || !d) return null;
  return new Date(y, m - 1, d);
}

export function addDays(d: Date, n: number): Date {
  const x = new Date(d);
  x.setDate(x.getDate() + n);
  return x;
}

export function dayDiff(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / DAY_MS);
}

export function today(): Date {
  const n = new Date();
  return new Date(n.getFullYear(), n.getMonth(), n.getDate());
}

export function fmtDay(d: Date, locale: string): string {
  return d.toLocaleDateString(locale, { month: "short", day: "numeric" });
}

/** 卡片的起止（未排期返回 null）。只有一端时按 1 天处理。 */
export function cardRange(card: ProjectCard): { start: Date; end: Date } | null {
  const s = parseDay(card.start_date);
  const e = parseDay(card.due_date);
  if (!s && !e) return null;
  return { start: s ?? e!, end: e ?? s! };
}

export const BAR_COLORS = [
  "bg-blue-500/80",
  "bg-emerald-500/80",
  "bg-amber-500/80",
  "bg-violet-500/80",
  "bg-rose-500/80",
  "bg-cyan-500/80",
  "bg-orange-500/80",
  "bg-teal-500/80",
];

export function cardLabel(card: ProjectCard): string {
  if (card.issue_number) return card.issue_title || `#${card.issue_number}`;
  return card.note || "";
}

// ---- 列表视图 ----
