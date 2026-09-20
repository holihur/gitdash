import { Bot, CheckCircle2, Circle, Flag, Pencil, Pin, PinOff, Trash2 } from "lucide-react";
import type { Issue, Label, Milestone } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { cn, formatDate } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import LabelChip from "@/components/label-chip";
import { MarkdownView } from "@/components/markdown";
import CommentSection from "@/components/comment-section";

export interface IssueDraft {
  labels: number[];
  milestone: number; // 0 = 无
}

/** 单条 issue（含展开后的正文、标签/里程碑编辑与评论） */
export function IssueItem({
  issue,
  openDetail,
  busy,
  draft,
  labels,
  milestones,
  canWrite,
  savingMeta,
  metaChanged,
  owner,
  name,
  onToggleExpand,
  onToggleState,
  onTogglePin,
  onEdit,
  onFixWithCopilot,
  onDelete,
  onToggleLabel,
  onSetMilestone,
  onSaveMeta,
}: {
  issue: Issue;
  openDetail: boolean;
  busy: boolean;
  draft: IssueDraft;
  labels: Label[];
  milestones: Milestone[];
  canWrite: boolean;
  savingMeta: boolean;
  metaChanged: boolean;
  owner: string;
  name: string;
  onToggleExpand: () => void;
  onToggleState: () => void;
  onTogglePin: () => void;
  onEdit: () => void;
  onFixWithCopilot: () => void;
  onDelete: () => void;
  onToggleLabel: (id: number) => void;
  onSetMilestone: (id: number) => void;
  onSaveMeta: () => void;
}) {
  const { t, lang } = useI18n();
  const locale = dateLocale(lang);
  const isOpen = issue.state === "open";
  const issueLabels = issue.labels ?? [];

  return (
    <div>
      <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:gap-3">
        <button
          type="button"
          className="flex min-w-0 flex-1 items-start gap-2 text-left"
          onClick={onToggleExpand}
        >
          {isOpen ? (
            <Circle className="mt-0.5 h-4 w-4 shrink-0 fill-green-500 text-green-600" />
          ) : (
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
          )}
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-1">
              {issue.pinned && (
                <Pin className="h-3.5 w-3.5 shrink-0 fill-current text-amber-500" />
              )}
              <span
                className={cn(
                  "min-w-0 truncate font-medium hover:underline",
                  !isOpen && "text-muted-foreground",
                )}
              >
                {issue.title}
              </span>
            </span>
            {(issueLabels.length > 0 || issue.milestone) && (
              <span className="mt-1 flex flex-wrap items-center gap-1">
                {issueLabels.map((l) => (
                  <LabelChip key={l.id} label={l} />
                ))}
                {issue.milestone && (
                  <span
                    className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs text-muted-foreground"
                    title={issue.milestone.description || issue.milestone.title}
                  >
                    <Flag className="h-3 w-3" />
                    <span className="max-w-40 truncate">{issue.milestone.title}</span>
                  </span>
                )}
              </span>
            )}
            <span className="mt-0.5 block truncate text-xs text-muted-foreground">
              #{issue.number} ·{" "}
              {isOpen
                ? t("issues.openedOn", {
                    author: issue.author,
                    date: formatDate(issue.created_at, locale),
                  })
                : t("issues.closedOn", {
                    author: issue.author,
                    date: formatDate(issue.closed_at ?? issue.updated_at, locale),
                  })}
            </span>
          </span>
        </button>
        <div className="flex shrink-0 items-center gap-2 pl-6 sm:pl-0">
          <Button
            size="sm"
            variant="outline"
            className="shrink-0"
            disabled={busy}
            onClick={onToggleState}
          >
            {isOpen ? t("issues.close") : t("issues.reopen")}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-8 w-8 shrink-0 p-0"
            title={issue.pinned ? t("issues.unpin") : t("issues.pin")}
            disabled={busy}
            onClick={onTogglePin}
          >
            {issue.pinned ? (
              <PinOff className="h-3.5 w-3.5" />
            ) : (
              <Pin className="h-3.5 w-3.5" />
            )}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-8 w-8 shrink-0 p-0"
            title={t("issues.edit")}
            onClick={onEdit}
          >
            <Pencil className="h-3.5 w-3.5" />
          </Button>
          {canWrite && (
            <Button
              size="sm"
              variant="ghost"
              className="h-8 w-8 shrink-0 p-0"
              title={t("issues.fixWithCopilot")}
              onClick={onFixWithCopilot}
            >
              <Bot className="h-3.5 w-3.5" />
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            className="h-8 w-8 shrink-0 p-0 text-destructive"
            title={t("issues.delete")}
            onClick={onDelete}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      {openDetail && (
        <div className="border-t bg-muted/30 px-4 py-4">
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_17rem]">
            {/* 左栏：issue 正文与评论 */}
            <div className="min-w-0 space-y-4">
              <div className="rounded-lg border bg-card p-4">
                {issue.body.trim() ? (
                  <MarkdownView text={issue.body} />
                ) : (
                  <p className="text-sm text-muted-foreground">{t("issues.noBody")}</p>
                )}
              </div>
              <CommentSection owner={owner} name={name} number={issue.number} />
            </div>

            {/* 右栏：标签 / 里程碑设置 */}
            <aside className="self-start">
              <div className="space-y-4 rounded-lg border bg-card p-3">
                <div>
                  <p className="mb-2 text-xs font-medium text-muted-foreground">
                    {t("issues.labels")}
                  </p>
                  {labels.length === 0 ? (
                    <p className="text-xs text-muted-foreground">{t("labels.emptyHint")}</p>
                  ) : (
                    <div className="flex flex-wrap gap-1.5">
                      {labels.map((l) => {
                        const selected = draft.labels.includes(l.id);
                        return (
                          <button
                            key={l.id}
                            type="button"
                            onClick={() => onToggleLabel(l.id)}
                            className={cn(
                              "rounded-full outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring",
                              selected ? "ring-2 ring-ring ring-offset-1" : "opacity-50 hover:opacity-80",
                            )}
                          >
                            <LabelChip label={l} />
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>

                <div className="space-y-1.5">
                  <p className="text-xs font-medium text-muted-foreground">
                    {t("issues.milestone")}
                  </p>
                  <select
                    value={draft.milestone || ""}
                    onChange={(e) => onSetMilestone(Number(e.target.value) || 0)}
                    className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="">{t("issues.noMilestone")}</option>
                    {milestones.map((m) => (
                      <option key={m.id} value={m.id} disabled={m.state === "closed"}>
                        {m.title}
                        {m.state === "closed" ? ` · ${t("issues.closed")}` : ""}
                      </option>
                    ))}
                  </select>
                </div>

                <Button
                  size="sm"
                  className="w-full"
                  disabled={savingMeta || !metaChanged}
                  onClick={onSaveMeta}
                >
                  {t("issues.saveMeta")}
                </Button>
              </div>
            </aside>
          </div>
        </div>
      )}
    </div>
  );
}
