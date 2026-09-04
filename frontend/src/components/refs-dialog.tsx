import { useState } from "react";
import { toast } from "sonner";
import { GitBranch, Plus, Tag, Trash2 } from "lucide-react";
import { api, type Tag as TagType } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

interface Props {
  open: boolean;
  onClose: () => void;
  owner: string;
  repo: string;
  branches: string[];
  head: string;
  tags: TagType[];
  current: string;
  onRefresh: () => void;
}

export default function RefsDialog({ open, onClose, owner, repo, branches, head, tags, current, onRefresh }: Props) {
  const { t, to } = useI18n();
  const [branchName, setBranchName] = useState("");
  const [tagName, setTagName] = useState("");
  const [busy, setBusy] = useState(false);

  const create = async (kind: "branch" | "tag") => {
    const name = kind === "branch" ? branchName.trim() : tagName.trim();
    if (!name) return;
    setBusy(true);
    try {
      await api.createRef(owner, repo, kind, name, current || "HEAD");
      toast.success(t("refs.created", { type: kind, name }));
      if (kind === "branch") setBranchName("");
      else setTagName("");
      onRefresh();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (kind: "branch" | "tag", name: string) => {
    setBusy(true);
    try {
      await api.deleteRef(owner, repo, kind, name);
      toast.success(t("refs.deleted", { type: kind, name }));
      onRefresh();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const fromRow = (label: string, placeholder: string, value: string, setValue: (s: string) => void, act: () => void, from?: string) => (
    <div className="grid gap-2">
      <Label>{label}</Label>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          placeholder={placeholder}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && act()}
          className="font-mono"
        />
        <Button onClick={act} disabled={busy || !value.trim()} className="gap-1.5">
          <Plus className="h-4 w-4" />
          {t("refs.create")}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t("refs.from")}: {from}</p>
    </div>
  );

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-[calc(100vw-2rem)] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {owner}/{repo} · {t("refs.manage")}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-5">
          <div className="space-y-2">
            <p className="flex items-center gap-2 text-sm font-medium">
              <GitBranch className="h-4 w-4" />
              {t("refs.branches")}
            </p>
            <div className="divide-y divide-border rounded-lg border">
              {branches.map((b) => (
                <div key={b} className="flex items-center gap-2 px-3 py-2">
                  <GitBranch className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <code className={cn("min-w-0 flex-1 truncate text-sm", b === current && "font-medium")}>
                    {b}
                  </code>
                  {b === head && (
                    <Badge variant="secondary" className="shrink-0">HEAD</Badge>
                  )}
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 shrink-0 text-destructive hover:text-destructive"
                    disabled={busy || b === head}
                    title={t("refs.deleteBranch")}
                    onClick={() => remove("branch", b)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              ))}
            </div>
            {fromRow(t("refs.branchName"), "feature/xxx", branchName, setBranchName, () => create("branch"), current || "HEAD")}
          </div>

          <div className="space-y-2">
            <p className="flex items-center gap-2 text-sm font-medium">
              <Tag className="h-4 w-4" />
              {t("refs.tags")}
            </p>
            {tags.length === 0 ? (
              <p className="rounded-lg border border-dashed py-4 text-center text-sm text-muted-foreground">
                {t("refs.noTags")}
              </p>
            ) : (
              <div className="divide-y divide-border rounded-lg border">
                {tags.map((tg) => (
                  <div key={tg.name} className="flex items-center gap-2 px-3 py-2">
                    <Tag className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <code className="min-w-0 flex-1 truncate text-sm">{tg.name}</code>
                    <span className="shrink-0 text-xs text-muted-foreground">{tg.sha.slice(0, 7)}</span>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 shrink-0 text-destructive hover:text-destructive"
                      disabled={busy}
                      title={t("refs.deleteTag")}
                      onClick={() => remove("tag", tg.name)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                ))}
              </div>
            )}
            {fromRow(t("refs.tagName"), "v1.0.0", tagName, setTagName, () => create("tag"), current || "HEAD")}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
