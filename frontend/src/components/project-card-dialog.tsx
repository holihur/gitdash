import { useEffect, useMemo, useState } from "react";
import type { Label, ProjectCard, ProjectColumn, ProjectSwimlane } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label as FieldLabel } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { MarkdownView } from "@/components/markdown";
import LabelChip from "@/components/label-chip";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

/** 可关联的 issue 选项（由上层加载后传入）。 */
export interface IssueOption {
  number: number;
  title: string;
  state: string;
}

/** 卡片表单草稿：名称 + Markdown 详情 + 日程 + 目标列/泳道 + 负责人/标签/关联 issue。 */
export interface CardDraft {
  title: string;
  body: string;
  issue_number: number;
  start_date: string;
  due_date: string;
  column_id: number;
  swimlane_id: number;
  assignees: string[];
  label_ids: number[];
}

const EMPTY: CardDraft = {
  title: "",
  body: "",
  issue_number: 0,
  start_date: "",
  due_date: "",
  column_id: 0,
  swimlane_id: 0,
  assignees: [],
  label_ids: [],
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
  people,
  labels,
  issues,
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
  /** 可指派的用户名（owner + 协作者） */
  people: string[];
  /** 仓库标签 */
  labels: Label[];
  /** 可关联的 issue */
  issues: IssueOption[];
  /** 新建时默认选中的列 / 泳道 */
  defaultTarget?: CardTarget | null;
  busy: boolean;
  onClose: () => void;
  onSubmit: (draft: CardDraft) => void;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState<CardDraft>(EMPTY);
  const [preview, setPreview] = useState(false);
  const [issueQuery, setIssueQuery] = useState("");

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
        assignees: card.assignees ?? [],
        label_ids: (card.labels ?? []).map((l) => l.id),
      });
    } else {
      setDraft({
        ...EMPTY,
        column_id: defaultTarget?.columnId ?? columns[0]?.id ?? 0,
        swimlane_id: defaultTarget?.swimlaneId ?? swimlanes[0]?.id ?? 0,
      });
    }
    setPreview(false);
    setIssueQuery("");
  }, [open, mode, card, defaultTarget, columns, swimlanes]);

  const hasIssue = draft.issue_number > 0;
  const invalidRange =
    draft.start_date !== "" && draft.due_date !== "" && draft.due_date < draft.start_date;
  const canSubmit = (hasIssue || draft.title.trim() !== "") && (mode === "edit" || draft.column_id > 0);

  const filteredIssues = useMemo(() => {
    const q = issueQuery.trim().toLowerCase();
    const list = q
      ? issues.filter((i) => String(i.number).includes(q) || i.title.toLowerCase().includes(q))
      : issues;
    return list.slice(0, 50);
  }, [issues, issueQuery]);

  const toggleAssignee = (u: string) =>
    setDraft((d) => ({
      ...d,
      assignees: d.assignees.includes(u) ? d.assignees.filter((x) => x !== u) : [...d.assignees, u],
    }));

  const toggleLabel = (id: number) =>
    setDraft((d) => ({
      ...d,
      label_ids: d.label_ids.includes(id) ? d.label_ids.filter((x) => x !== id) : [...d.label_ids, id],
    }));

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-[calc(100vw-2rem)] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {mode === "create" ? t("projects.addCard") : t("projects.editCard")}
            {hasIssue ? ` · #${draft.issue_number}` : ""}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <FieldLabel htmlFor="card-issue-search">{t("projects.cardIssueNumber")}</FieldLabel>
            <Input
              id="card-issue-search"
              placeholder={t("projects.issueSearch")}
              value={issueQuery}
              onChange={(e) => setIssueQuery(e.target.value)}
            />
            <select
              aria-label={t("projects.cardIssueNumber")}
              className="h-9 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              value={draft.issue_number}
              onChange={(e) => setDraft({ ...draft, issue_number: Number(e.target.value) || 0 })}
            >
              <option value={0}>{t("projects.noIssue")}</option>
              {filteredIssues.map((i) => (
                <option key={i.number} value={i.number}>
                  #{i.number} {i.title}
                </option>
              ))}
            </select>
          </div>

          {!hasIssue && (
            <div className="grid gap-1.5">
              <FieldLabel htmlFor="card-title">{t("projects.cardName")}</FieldLabel>
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
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <FieldLabel htmlFor="card-column">{t("projects.colColumn")}</FieldLabel>
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
                <FieldLabel htmlFor="card-swimlane">{t("projects.colSwimlane")}</FieldLabel>
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

          {!hasIssue && (
            <div className="grid gap-1.5">
              <div className="flex items-center justify-between">
                <FieldLabel htmlFor="card-body">{t("projects.cardDetails")}</FieldLabel>
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

          {people.length > 0 && (
            <div className="grid gap-1.5">
              <FieldLabel>{t("issues.assignees")}</FieldLabel>
              <div className="flex flex-wrap gap-1.5">
                {people.map((u) => {
                  const on = draft.assignees.includes(u);
                  return (
                    <button
                      key={u}
                      type="button"
                      onClick={() => toggleAssignee(u)}
                      className={
                        "rounded-full border px-2 py-0.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring " +
                        (on ? "border-primary text-foreground" : "text-muted-foreground opacity-60 hover:opacity-100")
                      }
                    >
                      {u}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {labels.length > 0 && (
            <div className="grid gap-1.5">
              <FieldLabel>{t("issues.labels")}</FieldLabel>
              <div className="flex flex-wrap gap-1.5">
                {labels.map((l) => {
                  const on = draft.label_ids.includes(l.id);
                  return (
                    <button
                      key={l.id}
                      type="button"
                      onClick={() => toggleLabel(l.id)}
                      className={
                        "rounded-full outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring " +
                        (on ? "ring-2 ring-ring ring-offset-1" : "opacity-50 hover:opacity-80")
                      }
                    >
                      <LabelChip label={l} />
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <FieldLabel htmlFor="card-start">{t("projects.startDate")}</FieldLabel>
              <Input
                id="card-start"
                type="date"
                value={draft.start_date}
                onChange={(e) => setDraft({ ...draft, start_date: e.target.value })}
              />
            </div>
            <div className="grid gap-1.5">
              <FieldLabel htmlFor="card-due">{t("projects.dueDate")}</FieldLabel>
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
