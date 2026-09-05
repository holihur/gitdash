import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, CheckCircle2, Circle, Flag, Pencil, Plus, Trash2, X } from "lucide-react";
import { api, type Milestone } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label as FieldLabel } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import ConfirmDialog from "@/components/confirm-dialog";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  owner: string;
  repo: string;
  onChanged?: () => void;
}

export default function MilestonesManager({ open, onOpenChange, owner, repo, onChanged }: Props) {
  const { t, to } = useI18n();
  const [items, setItems] = useState<Milestone[]>([]);
  const [busy, setBusy] = useState(false);

  // 新增
  const [title, setTitle] = useState("");
  const [desc, setDesc] = useState("");
  // 行内编辑
  const [editId, setEditId] = useState<number | null>(null);
  const [editTitle, setEditTitle] = useState("");
  const [editDesc, setEditDesc] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Milestone | null>(null);

  const load = useCallback(async () => {
    try {
      setItems(await api.listMilestones(owner, repo));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, repo, to]);

  useEffect(() => {
    if (open) {
      setItems([]);
      load();
      setEditId(null);
    }
  }, [open, load]);

  const add = async () => {
    if (!title.trim()) return;
    setBusy(true);
    try {
      await api.createMilestone(owner, repo, title.trim(), desc.trim());
      toast.success(t("milestones.added"));
      setTitle("");
      setDesc("");
      load();
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const startEdit = (m: Milestone) => {
    setEditId(m.id);
    setEditTitle(m.title);
    setEditDesc(m.description);
  };

  const saveEdit = async (m: Milestone) => {
    if (!editTitle.trim()) return;
    setBusy(true);
    try {
      await api.updateMilestone(owner, repo, m.id, {
        title: editTitle.trim(),
        description: editDesc.trim(),
        state: m.state,
      });
      toast.success(t("milestones.saved"));
      setEditId(null);
      load();
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const setState = async (m: Milestone, state: "open" | "closed") => {
    setBusy(true);
    try {
      await api.updateMilestone(owner, repo, m.id, { state });
      toast.success(t(state === "closed" ? "milestones.closed" : "milestones.reopened"));
      load();
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (m: Milestone) => {
    setPendingDelete(null);
    setBusy(true);
    try {
      await api.deleteMilestone(owner, repo, m.id);
      toast.success(t("milestones.deleted"));
      load();
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const total = (m: Milestone) => m.open_issues + m.closed_issues;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="truncate">
            {owner}/{repo} · {t("milestones.manage")}
          </DialogTitle>
          <DialogDescription>{t("milestones.emptyHint")}</DialogDescription>
        </DialogHeader>

        <div className="max-h-[50vh] space-y-4 overflow-auto pr-1">
          {items.length === 0 ? (
            <p className="flex items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-sm text-muted-foreground">
              <Flag className="h-4 w-4" />
              {t("milestones.empty")}
            </p>
          ) : (
            <div className="divide-y divide-border rounded-lg border">
              {items.map((m) =>
                editId === m.id ? (
                  <div key={m.id} className="space-y-2 px-3 py-2">
                    <Input
                      value={editTitle}
                      maxLength={100}
                      placeholder={t("milestones.titlePlaceholder")}
                      onChange={(e) => setEditTitle(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && saveEdit(m)}
                    />
                    <Textarea
                      rows={2}
                      value={editDesc}
                      placeholder={t("milestones.descriptionPlaceholder")}
                      onChange={(e) => setEditDesc(e.target.value)}
                    />
                    <div className="flex items-center gap-1">
                      <Button size="sm" variant="outline" className="gap-1.5" onClick={() => saveEdit(m)} disabled={busy || !editTitle.trim()}>
                        <Check className="h-4 w-4" />
                        {t("common.save")}
                      </Button>
                      <Button size="sm" variant="ghost" className="gap-1.5" onClick={() => setEditId(null)}>
                        <X className="h-4 w-4" />
                        {t("common.cancel")}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div key={m.id} className="px-3 py-2">
                    <div className="flex items-start gap-2">
                      {m.state === "open" ? (
                        <Circle className="mt-0.5 h-4 w-4 shrink-0 fill-green-500 text-green-600" />
                      ) : (
                        <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                      )}
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-medium">{m.title}</p>
                        {m.description && (
                          <p className="line-clamp-2 text-xs text-muted-foreground">{m.description}</p>
                        )}
                        <p className="text-xs text-muted-foreground">
                          {t("milestones.counts", {
                            open: m.open_issues,
                            closed: m.closed_issues,
                          })}
                          {total(m) > 0 && (
                            <span className="ml-2 inline-block h-1.5 w-24 overflow-hidden rounded-full bg-muted align-middle">
                              <span
                                className="block h-full bg-green-500"
                                style={{ width: `${(m.closed_issues / total(m)) * 100}%` }}
                              />
                            </span>
                          )}
                        </p>
                      </div>
                      <div className="flex shrink-0 items-center gap-0.5">
                        {m.state === "open" ? (
                          <Button variant="ghost" size="sm" className="h-8 px-2" disabled={busy} onClick={() => setState(m, "closed")}>
                            {t("issues.close")}
                          </Button>
                        ) : (
                          <Button variant="ghost" size="sm" className="h-8 px-2" disabled={busy} onClick={() => setState(m, "open")}>
                            {t("issues.reopen")}
                          </Button>
                        )}
                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => startEdit(m)} title={t("milestones.edit")}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-destructive hover:text-destructive"
                          onClick={() => setPendingDelete(m)}
                          title={t("milestones.remove")}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  </div>
                ),
              )}
            </div>
          )}

          <div className="space-y-2 rounded-lg border bg-muted/30 p-3">
            <div className="grid gap-2">
              <FieldLabel htmlFor="ms-title">{t("milestones.titleLabel")}</FieldLabel>
              <Input
                id="ms-title"
                placeholder={t("milestones.titlePlaceholder")}
                maxLength={100}
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && add()}
              />
            </div>
            <div className="grid gap-2">
              <FieldLabel htmlFor="ms-desc">{t("milestones.descriptionLabel")}</FieldLabel>
              <Textarea
                id="ms-desc"
                rows={2}
                placeholder={t("milestones.descriptionPlaceholder")}
                value={desc}
                onChange={(e) => setDesc(e.target.value)}
              />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={add} disabled={busy || !title.trim()}>
            <Plus className="h-4 w-4" />
            {t("milestones.add")}
          </Button>
        </DialogFooter>

        <ConfirmDialog
          open={pendingDelete !== null}
          onOpenChange={(o) => !o && setPendingDelete(null)}
          description={t("milestones.confirmDelete", { title: pendingDelete?.title ?? "" })}
          onConfirm={() => pendingDelete && remove(pendingDelete)}
          busy={busy}
        />
      </DialogContent>
    </Dialog>
  );
}
