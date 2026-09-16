import { useEffect, useState } from "react";
import type { ProjectCard } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";


export interface CardDraft {
  note: string;
  start_date: string;
  due_date: string;
}

export function ProjectCardDialog({
  card,
  onClose,
  onSave,
  busy,
}: {
  card: ProjectCard | null;
  onClose: () => void;
  onSave: (card: ProjectCard, draft: CardDraft) => void;
  busy: boolean;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState<CardDraft>({ note: "", start_date: "", due_date: "" });

  // 打开卡片时重置表单
  useEffect(() => {
    if (card) {
      setDraft({ note: card.note ?? "", start_date: card.start_date ?? "", due_date: card.due_date ?? "" });
    }
  }, [card]);

  const invalidRange = draft.start_date !== "" && draft.due_date !== "" && draft.due_date < draft.start_date;

  return (
    <Dialog open={card !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t("projects.editCard")}
            {card?.issue_number ? ` · #${card.issue_number}` : ""}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          {!card?.issue_number && (
            <div className="grid gap-1.5">
              <Label htmlFor="card-note">{t("projects.note")}</Label>
              <Textarea id="card-note" rows={3} value={draft.note} onChange={(e) => setDraft({ ...draft, note: e.target.value })} />
            </div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="card-start">{t("projects.startDate")}</Label>
              <Input id="card-start" type="date" value={draft.start_date} onChange={(e) => setDraft({ ...draft, start_date: e.target.value })} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="card-due">{t("projects.dueDate")}</Label>
              <Input id="card-due" type="date" value={draft.due_date} onChange={(e) => setDraft({ ...draft, due_date: e.target.value })} />
            </div>
          </div>
          {invalidRange && <p className="text-xs text-destructive">{t("projects.invalidRange")}</p>}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button onClick={() => card && onSave(card, draft)} disabled={busy || invalidRange}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
