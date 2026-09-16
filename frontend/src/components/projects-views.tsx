import { useMemo } from "react";
import { Pencil, Trash2 } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { UNGROUPED, parseDay, today, type ProjectViewProps } from "@/components/project-view-shared";

export type { ProjectViewProps } from "@/components/project-view-shared";
export { ProjectGanttView } from "@/components/project-gantt";
export { ProjectCardDialog, type CardDraft } from "@/components/project-card-dialog";

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

