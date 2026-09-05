import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, Pencil, Plus, Tag, Trash2, X } from "lucide-react";
import { api, type Label } from "@/lib/api";
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
import LabelChip, { labelTextColor } from "@/components/label-chip";
import ConfirmDialog from "@/components/confirm-dialog";

const HEX_RE = /^#?[0-9a-fA-F]{6}$/;

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  owner: string;
  repo: string;
  /** 有改动后通知父级刷新（issue 行上的标签）。 */
  onChanged?: () => void;
}

function ColorInput({
  id,
  value,
  onChange,
}: {
  id: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <input
        type="color"
        className="h-8 w-10 shrink-0 cursor-pointer rounded border border-input bg-transparent p-0.5"
        value={/^#[0-9a-fA-F]{6}$/.test(value) ? value : "#0366d6"}
        onChange={(e) => onChange(e.target.value.slice(1))}
      />
      <Input
        id={id}
        className="font-mono text-xs"
        placeholder="0366d6"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}

export default function LabelsManager({ open, onOpenChange, owner, repo, onChanged }: Props) {
  const { t, to } = useI18n();
  const [labels, setLabels] = useState<Label[]>([]);
  const [busy, setBusy] = useState(false);

  // 新增
  const [name, setName] = useState("");
  const [color, setColor] = useState("0366d6");
  // 行内编辑
  const [editId, setEditId] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [editColor, setEditColor] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Label | null>(null);

  const load = useCallback(async () => {
    try {
      setLabels(await api.listLabels(owner, repo));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, repo, to]);

  useEffect(() => {
    if (open) {
      setLabels([]);
      load();
      setEditId(null);
    }
  }, [open, load]);

  const valid = (n: string, c: string): string | null => {
    if (!n.trim()) return t("labels.nameRequired");
    if (n.trim().length > 50) return t("labels.nameTooLong");
    if (!HEX_RE.test(c.trim())) return t("labels.colorInvalid");
    return null;
  };

  const add = async () => {
    const err = valid(name, color);
    if (err) {
      toast.error(err);
      return;
    }
    setBusy(true);
    try {
      await api.createLabel(owner, repo, name.trim(), color.trim());
      toast.success(t("labels.added"));
      setName("");
      setColor("0366d6");
      load();
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const startEdit = (l: Label) => {
    setEditId(l.id);
    setEditName(l.name);
    setEditColor(l.color);
  };

  const saveEdit = async (l: Label) => {
    const err = valid(editName, editColor);
    if (err) {
      toast.error(err);
      return;
    }
    setBusy(true);
    try {
      await api.updateLabel(owner, repo, l.id, editName.trim(), editColor.trim());
      toast.success(t("labels.saved"));
      setEditId(null);
      load();
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (l: Label) => {
    setPendingDelete(null);
    setBusy(true);
    try {
      await api.deleteLabel(owner, repo, l.id);
      toast.success(t("labels.deleted"));
      load();
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const preview: Label = { id: 0, name: name || " " , color: color.replace(/^#/, "") };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="truncate">
            {owner}/{repo} · {t("labels.manage")}
          </DialogTitle>
          <DialogDescription>{t("labels.emptyHint")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {labels.length === 0 ? (
            <p className="flex items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-sm text-muted-foreground">
              <Tag className="h-4 w-4" />
              {t("labels.empty")}
            </p>
          ) : (
            <div className="divide-y divide-border rounded-lg border">
              {labels.map((l) =>
                editId === l.id ? (
                  <div key={l.id} className="space-y-2 px-3 py-2">
                    <Input
                      value={editName}
                      maxLength={50}
                      onChange={(e) => setEditName(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && saveEdit(l)}
                    />
                    <div className="flex items-center gap-1">
                      <div className="min-w-0 flex-1">
                        <ColorInput id={`lbl-edit-${l.id}`} value={editColor} onChange={setEditColor} />
                      </div>
                      <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0" onClick={() => saveEdit(l)} title={t("common.save")}>
                        <Check className="h-4 w-4" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0" onClick={() => setEditId(null)} title={t("common.cancel")}>
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div key={l.id} className="flex items-center gap-2 px-3 py-2">
                    <LabelChip label={l} className="min-w-0" />
                    <div className="ml-auto flex shrink-0 items-center gap-1">
                      <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => startEdit(l)} title={t("labels.edit")}>
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => setPendingDelete(l)}
                        title={t("labels.remove")}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ),
              )}
            </div>
          )}

          <div className="space-y-2 rounded-lg border bg-muted/30 p-3">
            <div className="flex items-center gap-2">
              <FieldLabel htmlFor="lbl-name" className="shrink-0 text-sm">
                {t("common.name")}
              </FieldLabel>
              <div className="min-w-0 flex-1">
                <Input
                  id="lbl-name"
                  placeholder={t("labels.namePlaceholder")}
                  maxLength={50}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && add()}
                />
              </div>
              <span
                className="hidden h-6 shrink-0 items-center rounded-full px-2 text-xs font-medium sm:inline-flex"
                style={{
                  backgroundColor: "#" + preview.color,
                  color: labelTextColor(preview.color),
                }}
              >
                {name || t("labels.preview")}
              </span>
            </div>
            <div className="grid gap-2">
              <FieldLabel htmlFor="lbl-color">{t("labels.colorLabel")}</FieldLabel>
              <ColorInput id="lbl-color" value={color} onChange={setColor} />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={add} disabled={busy || !name.trim()}>
            <Plus className="h-4 w-4" />
            {t("labels.add")}
          </Button>
        </DialogFooter>

        <ConfirmDialog
          open={pendingDelete !== null}
          onOpenChange={(o) => !o && setPendingDelete(null)}
          description={t("labels.confirmDelete", { name: pendingDelete?.name ?? "" })}
          onConfirm={() => pendingDelete && remove(pendingDelete)}
          busy={busy}
        />
      </DialogContent>
    </Dialog>
  );
}
