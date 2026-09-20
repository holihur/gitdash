import { useEffect, useState } from "react";
import type { ProjectCard, ProjectColumn, ProjectSwimlane } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { MarkdownView } from "@/components/markdown";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

/** 卡片表单草稿：名称 + Markdown 详情 + 日程 + 目标列/泳道。 */
export interface CardDraft {
  title: string;
  body: string;
  issue_number: number;
  start_date: string;
  due_date: string;
  column_id: number;
  swimlane_id: number;
}

const EMPTY: CardDraft = {
  title: "",
  body: "",
  issue_number: 0,
  start_date: "",
  due_date: "",
  column_id: 0,
  swimlane_id: 0,
};

/** 新建卡片的默认落位（看板单元格 / 列表、甘特视图的全局按钮）。 */
export interface CardTarget {
  columnId: number;
  swimlaneId: number;
}

export function ProjectCardDialog({
  open,
  mode,
  card,
  columns,
  swimlanes,
  defaultTarget,
  busy,
  onClose,
  onSubmit,
}: {
  open: boolean;
  mode: "create" | "edit";
  card?: ProjectCard | null;
  /** 新建时可选的目标列 / 泳道 */
  columns: ProjectColumn[];
  swimlanes: ProjectSwimlane[];
  /** 新建时默认选中的列 / 泳道 */
  defaultTarget?: CardTarget | null;
  busy: boolean;
  onClose: () => void;
  onSubmit: (draft: CardDraft) => void;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState<CardDraft>(EMPTY);
  const [preview, setPreview] = useState(false);

  // 打开时按模式重置表单
  useEffect(() => {
    if (!open) return;
    if (mode === "edit" && card) {
      setDraft({
        title: card.title ?? card.note ?? "",
        body: card.body ?? "",
        issue_number: card.issue_number ?? 0,
        start_date: card.start_date ?? "",
        due_date: card.due_date ?? "",
        column_id: card.column_id,
        swimlane_id: card.swimlane_id,
      });
    } else {
      setDraft({
        ...EMPTY,
        column_id: defaultTarget?.columnId ?? columns[0]?.id ?? 0,
        swimlane_id: defaultTarget?.swimlaneId ?? swimlanes[0]?.id ?? 0,
      });
    }
    setPreview(false);
  }, [open, mode, card, defaultTarget, columns, swimlanes]);

  // issue 卡片的名称/详情由 issue 本身承载，只能编辑日程。
  const isIssue = mode === "edit" && !!card?.issue_number;
  const invalidRange =
    draft.start_date !== "" && draft.due_date !== "" && draft.due_date < draft.start_date;
  const canSubmit =
    isIssue ||
    (draft.column_id > 0 && (draft.title.trim() !== "" || draft.issue_number > 0));

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {mode === "create" ? t("projects.addCard") : t("projects.editCard")}
            {isIssue ? ` · #${card?.issue_number}` : ""}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          {!isIssue && (
            <div className="grid gap-1.5">
              <Label htmlFor="card-title">{t("projects.cardName")}</Label>
              <Input
                id="card-title"
                maxLength={200}
                placeholder={t("projects.cardNamePlaceholder")}
                value={draft.title}
                onChange={(e) => setDraft({ ...draft, title: e.target.value })}
              />
            </div>
          )}
          {mode === "create" && (
            <div className="grid gap-1.5">
              <Label htmlFor="card-issue">{t("projects.cardIssueNumber")}</Label>
              <Input
                id="card-issue"
                inputMode="numeric"
                className="h-8 w-28"
                placeholder={t("projects.cardIssueHint")}
                value={draft.issue_number > 0 ? String(draft.issue_number) : ""}
                onChange={(e) => {
                  const n = Number(e.target.value.replace(/[^0-9]/g, ""));
                  setDraft({ ...draft, issue_number: Number.isFinite(n) ? n : 0 });
                }}
              />
            </div>
          )}
          {mode === "create" && (
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="card-column">{t("projects.colColumn")}</Label>
                <select
                  id="card-column"
                  className="h-9 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={draft.column_id}
                  onChange={(e) => setDraft({ ...draft, column_id: Number(e.target.value) })}
                >
                  {columns.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="card-swimlane">{t("projects.colSwimlane")}</Label>
                <select
                  id="card-swimlane"
                  className="h-9 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={draft.swimlane_id}
                  onChange={(e) => setDraft({ ...draft, swimlane_id: Number(e.target.value) })}
                >
                  <option value={0}>{t("projects.ungrouped")}</option>
                  {swimlanes.map((l) => (
                    <option key={l.id} value={l.id}>
                      {l.name}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          )}
          {!isIssue && (
            <div className="grid gap-1.5">
              <div className="flex items-center justify-between">
                <Label htmlFor="card-body">{t("projects.cardDetails")}</Label>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={() => setPreview((p) => !p)}
                >
                  {preview ? t("projects.cardEditBody") : t("projects.cardPreview")}
                </Button>
              </div>
              {preview ? (
                <div className="min-h-24 rounded-md border px-3 py-2">
                  {draft.body.trim() ? (
                    <MarkdownView text={draft.body} className="text-sm" />
                  ) : (
                    <p className="text-sm text-muted-foreground">{t("projects.cardNoDetails")}</p>
                  )}
                </div>
              ) : (
                <Textarea
                  id="card-body"
                  rows={5}
                  placeholder={t("projects.cardDetailsHint")}
                  value={draft.body}
                  onChange={(e) => setDraft({ ...draft, body: e.target.value })}
                />
              )}
            </div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="card-start">{t("projects.startDate")}</Label>
              <Input
                id="card-start"
                type="date"
                value={draft.start_date}
                onChange={(e) => setDraft({ ...draft, start_date: e.target.value })}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="card-due">{t("projects.dueDate")}</Label>
              <Input
                id="card-due"
                type="date"
                value={draft.due_date}
                onChange={(e) => setDraft({ ...draft, due_date: e.target.value })}
              />
            </div>
          </div>
          {invalidRange && <p className="text-xs text-destructive">{t("projects.invalidRange")}</p>}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button onClick={() => onSubmit(draft)} disabled={busy || invalidRange || !canSubmit}>
            {mode === "create" ? t("projects.addCard") : t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
