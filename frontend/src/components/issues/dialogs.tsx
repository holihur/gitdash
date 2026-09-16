import { Bot } from "lucide-react";
import type { ByokKey } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label as FieldLabel } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { MarkdownEditor } from "@/components/markdown-editor";
import { useI18n } from "@/lib/i18n";

/** 新建 issue 对话框 */
export function CreateIssueDialog({
  open,
  onOpenChange,
  title,
  onTitleChange,
  body,
  onBodyChange,
  busy,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  onTitleChange: (v: string) => void;
  body: string;
  onBodyChange: (v: string) => void;
  busy: boolean;
  onSubmit: () => void;
}) {
  const { t } = useI18n();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("issues.newDialogTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <FieldLabel htmlFor="issue-title">{t("issues.titleLabel")}</FieldLabel>
            <Input
              id="issue-title"
              placeholder={t("issues.titlePlaceholder")}
              maxLength={200}
              value={title}
              onChange={(e) => onTitleChange(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  onSubmit();
                }
              }}
            />
          </div>
          <div className="grid gap-2">
            <FieldLabel htmlFor="issue-body">{t("issues.bodyLabel")}</FieldLabel>
            <MarkdownEditor
              id="issue-body"
              rows={5}
              placeholder={t("issues.bodyPlaceholder")}
              value={body}
              onChange={onBodyChange}
            />
          </div>
        </div>
        <DialogFooter>
          <Button onClick={onSubmit} disabled={busy || !title.trim()}>
            {t("issues.new")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** 编辑 issue 对话框 */
export function EditIssueDialog({
  open,
  number,
  title,
  onTitleChange,
  body,
  onBodyChange,
  busy,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  number: number;
  title: string;
  onTitleChange: (v: string) => void;
  body: string;
  onBodyChange: (v: string) => void;
  busy: boolean;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("issues.editDialogTitle", { number })}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <FieldLabel htmlFor="edit-issue-title">{t("issues.titleLabel")}</FieldLabel>
            <Input
              id="edit-issue-title"
              maxLength={200}
              value={title}
              onChange={(e) => onTitleChange(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <FieldLabel htmlFor="edit-issue-body">{t("issues.bodyLabel")}</FieldLabel>
            <MarkdownEditor id="edit-issue-body" rows={6} value={body} onChange={onBodyChange} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button onClick={onSubmit} disabled={busy || !title.trim()}>
            {t("issues.saveEdit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** 「用 Copilot 修复 issue」对话框 */
export function CopilotLaunchDialog({
  open,
  number,
  byokKeys,
  byokId,
  onByokChange,
  note,
  onNoteChange,
  busy,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  number: number;
  byokKeys: ByokKey[] | null;
  byokId: number;
  onByokChange: (id: number) => void;
  note: string;
  onNoteChange: (v: string) => void;
  busy: boolean;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t("issues.fixWithCopilot")} · #{number}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-4">
          <p className="text-sm text-muted-foreground">{t("issues.fixWithCopilotHint")}</p>
          {byokKeys !== null && byokKeys.length === 0 ? (
            <p className="rounded-lg border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">
              {t("copilot.noByok")}
            </p>
          ) : (
            <>
              <div className="grid gap-2">
                <FieldLabel htmlFor="copilot-issue-byok">{t("copilot.byok")}</FieldLabel>
                <select
                  id="copilot-issue-byok"
                  className="h-9 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={byokId}
                  onChange={(e) => onByokChange(Number(e.target.value))}
                >
                  {(byokKeys ?? []).map((k) => (
                    <option key={k.id} value={k.id}>
                      {k.name} ({k.model || "claude"})
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-2">
                <FieldLabel htmlFor="copilot-issue-note">{t("copilot.prompt")}</FieldLabel>
                <Textarea
                  id="copilot-issue-note"
                  rows={3}
                  value={note}
                  onChange={(e) => onNoteChange(e.target.value)}
                  placeholder={t("copilot.promptPlaceholder")}
                />
              </div>
            </>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={onSubmit}
            disabled={busy || !byokId || (byokKeys !== null && byokKeys.length === 0)}
          >
            <Bot className="h-4 w-4" />
            {t("copilot.launch")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
