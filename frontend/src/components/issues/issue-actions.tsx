import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Bot, Pencil, Pin, PinOff, Trash2 } from "lucide-react";
import { api, type ByokKey, type Issue } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { MarkdownEditor } from "@/components/markdown-editor";
import ConfirmDialog from "@/components/confirm-dialog";
import { CopilotLaunchDialog, EditIssueDialog } from "@/components/issues/dialogs";

/**
 * issue 列表行与详情页共用的操作区：关闭/重开、置顶、编辑、用 Copilot 修复、删除。
 * 自带各自的对话框；变更成功后回调 onChanged（删除后若传 onDeleted 则优先调用）。
 */
export function IssueActions({
  owner,
  name,
  issue,
  canWrite,
  canTriage,
  onChanged,
  onDeleted,
}: {
  owner: string;
  name: string;
  issue: Issue;
  canWrite: boolean;
  canTriage: boolean;
  onChanged: () => void;
  onDeleted?: () => void;
}) {
  const { t, to } = useI18n();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);

  const [editOpen, setEditOpen] = useState(false);
  const [editTitle, setEditTitle] = useState(issue.title);
  const [editBody, setEditBody] = useState(issue.body);
  const [savingEdit, setSavingEdit] = useState(false);

  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const [copilotOpen, setCopilotOpen] = useState(false);
  const [byokKeys, setByokKeys] = useState<ByokKey[] | null>(null);
  const [copilotByokId, setCopilotByokId] = useState(0);
  const [copilotNote, setCopilotNote] = useState("");
  const [copilotBusy, setCopilotBusy] = useState(false);

  const isOpen = issue.state === "open";

  const [closeOpen, setCloseOpen] = useState(false);
  const [closeComment, setCloseComment] = useState("");
  const [closeReason, setCloseReason] = useState<"completed" | "not_planned">("completed");

  const toggleState = async () => {
    if (isOpen) {
      setCloseComment("");
      setCloseReason("completed");
      setCloseOpen(true);
      return;
    }
    setBusy(true);
    try {
      await api.setIssueState(owner, name, issue.number, "open");
      toast.success(t("issues.stateOpen", { number: issue.number }));
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const confirmClose = async () => {
    setBusy(true);
    try {
      await api.updateIssue(owner, name, issue.number, {
        state: "closed",
        comment: closeComment.trim() || undefined,
        state_reason: closeReason,
      });
      toast.success(t("issues.stateClosed", { number: issue.number }));
      setCloseOpen(false);
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const togglePin = async () => {
    setBusy(true);
    try {
      await api.updateIssue(owner, name, issue.number, { pinned: !issue.pinned });
      toast.success(
        issue.pinned
          ? t("issues.unpinned", { number: issue.number })
          : t("issues.pinnedMsg", { number: issue.number }),
      );
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const openEdit = () => {
    setEditTitle(issue.title);
    setEditBody(issue.body);
    setEditOpen(true);
  };

  const saveEdit = async () => {
    if (!editTitle.trim()) return;
    const patch: { title?: string; body?: string } = {};
    if (editTitle.trim() !== issue.title) patch.title = editTitle.trim();
    if (editBody !== issue.body) patch.body = editBody;
    if (Object.keys(patch).length === 0) {
      setEditOpen(false);
      return;
    }
    setSavingEdit(true);
    try {
      await api.updateIssue(owner, name, issue.number, patch);
      toast.success(t("issues.edited", { number: issue.number }));
      setEditOpen(false);
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSavingEdit(false);
    }
  };

  const remove = async () => {
    setDeleting(true);
    try {
      await api.deleteIssue(owner, name, issue.number);
      toast.success(t("issues.deleted", { number: issue.number }));
      setDeleteOpen(false);
      if (onDeleted) onDeleted();
      else onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setDeleting(false);
    }
  };

  const openCopilot = async () => {
    setCopilotOpen(true);
    setCopilotNote("");
    if (byokKeys === null) {
      try {
        const keys = await api.listByok();
        setByokKeys(keys);
        setCopilotByokId(keys[0]?.id ?? 0);
      } catch {
        setByokKeys([]);
      }
    } else {
      setCopilotByokId(byokKeys[0]?.id ?? 0);
    }
  };

  const launchCopilot = async () => {
    if (!copilotByokId) return;
    setCopilotBusy(true);
    try {
      const session = await api.createCopilot(owner, name, {
        byok_id: copilotByokId,
        issue_number: issue.number,
        prompt: copilotNote.trim(),
      });
      toast.success(t("copilot.launch"));
      setCopilotOpen(false);
      navigate(`/repo/${owner}/${name}/copilot?copilot=${session.id}`);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setCopilotBusy(false);
    }
  };

  return (
    <div className="flex shrink-0 items-center gap-2">
      {canTriage && (
        <Button size="sm" variant="outline" className="shrink-0" disabled={busy} onClick={toggleState}>
          {isOpen ? t("issues.close") : t("issues.reopen")}
        </Button>
      )}
      {canTriage && (
        <Button
          size="sm"
          variant="ghost"
          className="h-8 w-8 shrink-0 p-0"
          title={issue.pinned ? t("issues.unpin") : t("issues.pin")}
          disabled={busy}
          onClick={togglePin}
        >
          {issue.pinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
        </Button>
      )}
      {canTriage && (
        <Button
          size="sm"
          variant="ghost"
          className="h-8 w-8 shrink-0 p-0"
          title={t("issues.edit")}
          onClick={openEdit}
        >
          <Pencil className="h-3.5 w-3.5" />
        </Button>
      )}
      {canWrite && (
        <Button
          size="sm"
          variant="ghost"
          className="h-8 w-8 shrink-0 p-0"
          title={t("issues.fixWithCopilot")}
          onClick={openCopilot}
        >
          <Bot className="h-3.5 w-3.5" />
        </Button>
      )}
      {canTriage && (
        <Button
          size="sm"
          variant="ghost"
          className="h-8 w-8 shrink-0 p-0 text-destructive"
          title={t("issues.delete")}
          onClick={() => setDeleteOpen(true)}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      )}

      <EditIssueDialog
        open={editOpen}
        number={issue.number}
        title={editTitle}
        onTitleChange={setEditTitle}
        body={editBody}
        onBodyChange={setEditBody}
        busy={savingEdit}
        onSubmit={saveEdit}
        onCancel={() => setEditOpen(false)}
      />
      <CopilotLaunchDialog
        open={copilotOpen}
        number={issue.number}
        byokKeys={byokKeys}
        byokId={copilotByokId}
        onByokChange={setCopilotByokId}
        note={copilotNote}
        onNoteChange={setCopilotNote}
        busy={copilotBusy}
        onSubmit={launchCopilot}
        onCancel={() => setCopilotOpen(false)}
      />
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={(o) => !o && setDeleteOpen(false)}
        title={t("issues.deleteConfirmTitle")}
        description={t("issues.deleteConfirmDesc", { number: issue.number })}
        confirmText={t("common.delete")}
        busy={deleting}
        onConfirm={remove}
      />
      <Dialog open={closeOpen} onOpenChange={setCloseOpen}>
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("issues.closeWithComment")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3">
            <MarkdownEditor
              rows={3}
              placeholder={t("issues.closeCommentPlaceholder")}
              value={closeComment}
              onChange={setCloseComment}
            />
            <div className="grid gap-1.5">
              <span className="text-sm font-medium">{t("issues.closeReason")}</span>
              <select
                aria-label={t("issues.closeReason")}
                value={closeReason}
                onChange={(e) => setCloseReason(e.target.value as "completed" | "not_planned")}
                className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="completed">{t("issues.reasonCompleted")}</option>
                <option value="not_planned">{t("issues.reasonNotPlanned")}</option>
              </select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCloseOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={confirmClose} disabled={busy} data-testid="confirm-close">
              {t("issues.close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
