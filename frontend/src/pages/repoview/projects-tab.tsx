import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { KanbanSquare, Plus, Trash2 } from "lucide-react";
import { api, type Project } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label as FieldLabel } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import ConfirmDialog from "@/components/confirm-dialog";
import ProjectsBoard from "@/components/projects-board";

interface Props {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
}

export default function ProjectsTab({ owner, name, role }: Props) {
  const { t, to } = useI18n();
  const canWrite = role === "owner" || role === "write";
  const [items, setItems] = useState<Project[]>([]);
  const [current, setCurrent] = useState<Project | null>(null);
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [newDesc, setNewDesc] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Project | null>(null);

  const load = useCallback(async () => {
    try {
      setItems(await api.listProjects(owner, name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const add = async () => {
    if (!newName.trim()) return;
    setBusy(true);
    try {
      await api.createProject(owner, name, newName.trim(), newDesc.trim());
      toast.success(t("projects.added"));
      setOpen(false);
      setNewName("");
      setNewDesc("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!pendingDelete) return;
    setBusy(true);
    try {
      await api.deleteProject(owner, name, pendingDelete.id);
      toast.success(t("projects.deleted"));
      setPendingDelete(null);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  if (current) {
    return (
      <ProjectsBoard
        owner={owner}
        name={name}
        project={current}
        role={role}
        onBack={() => { setCurrent(null); load(); }}
        onProjectChanged={(p) => setCurrent(p)}
      />
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          {t("projects.count", { count: items.length })}
        </p>
        {canWrite && (
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button size="sm" className="gap-2">
                <Plus className="h-4 w-4" />
                {t("projects.add")}
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>{t("projects.add")}</DialogTitle>
              </DialogHeader>
              <div className="grid gap-4">
                <div className="grid gap-2">
                  <FieldLabel htmlFor="proj-name">{t("projects.nameLabel")}</FieldLabel>
                  <Input
                    id="proj-name"
                    placeholder={t("projects.namePlaceholder")}
                    maxLength={100}
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && add()}
                  />
                </div>
                <div className="grid gap-2">
                  <FieldLabel htmlFor="proj-desc">{t("projects.descriptionLabel")}</FieldLabel>
                  <Textarea
                    id="proj-desc"
                    rows={2}
                    placeholder={t("projects.descriptionPlaceholder")}
                    value={newDesc}
                    onChange={(e) => setNewDesc(e.target.value)}
                  />
                </div>
              </div>
              <DialogFooter>
                <Button onClick={add} disabled={busy || !newName.trim()}>
                  <Plus className="h-4 w-4" />
                  {t("projects.add")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        )}
      </div>

      {items.length === 0 ? (
        <p className="flex items-center justify-center gap-2 rounded-lg border border-dashed py-10 text-sm text-muted-foreground">
          <KanbanSquare className="h-4 w-4" />
          {t("projects.empty")}
        </p>
      ) : (
        <div className="divide-y divide-border rounded-lg border">
          {items.map((p) => (
            <div key={p.id} className="flex items-start gap-2 px-3 py-2">
              <button className="min-w-0 flex-1 text-left" onClick={() => setCurrent(p)}>
                <p className="truncate font-medium hover:underline">{p.name}</p>
                {p.description && (
                  <p className="line-clamp-2 text-xs text-muted-foreground">{p.description}</p>
                )}
                <p className="text-xs text-muted-foreground">
                  {t("projects.cardCount", { count: p.card_count })}
                </p>
              </button>
              {canWrite && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-destructive hover:text-destructive"
                  onClick={() => setPendingDelete(p)}
                  title={t("projects.remove")}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              )}
            </div>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("projects.confirmDelete", { name: pendingDelete?.name ?? "" })}
        onConfirm={remove}
        busy={busy}
      />
    </div>
  );
}
