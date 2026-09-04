import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Plus, Trash2, UserRound } from "lucide-react";
import { api, type Collab } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
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
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  owner: string;
  repo: string;
}

export default function CollaboratorsDialog({ open, onOpenChange, owner, repo }: Props) {
  const { t, to } = useI18n();
  const [collabs, setCollabs] = useState<Collab[]>([]);
  const [username, setUsername] = useState("");
  const [permission, setPermission] = useState<"read" | "write">("write");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setCollabs(await api.listCollabs(owner, repo));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, repo, to]);

  useEffect(() => {
    if (open) {
      setCollabs([]);
      load();
    }
  }, [open, load]);

  const add = async () => {
    if (!username.trim()) return;
    setBusy(true);
    try {
      await api.addCollab(owner, repo, username.trim(), permission);
      toast.success(t("collabs.added", { username: username.trim(), permission: t(`collabs.${permission}`) }));
      setUsername("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (c: Collab) => {
    setBusy(true);
    try {
      await api.removeCollab(owner, repo, c.username);
      toast.success(t("collabs.removed", { username: c.username }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="truncate">
            {owner}/{repo} · {t("collabs.manage")}
          </DialogTitle>
          <DialogDescription>{t("collabs.emptyHint")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {collabs.length === 0 ? (
            <p className="flex items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-sm text-muted-foreground">
              <UserRound className="h-4 w-4" />
              {t("collabs.empty")}
            </p>
          ) : (
            <div className="divide-y divide-border rounded-lg border">
              {collabs.map((c) => (
                <div key={c.username} className="flex items-center gap-3 px-3 py-2">
                  <span className="flex-1 truncate text-sm font-medium">{c.username}</span>
                  <Badge
                    variant={c.permission === "write" ? "default" : "secondary"}
                    className="shrink-0"
                  >
                    {c.permission === "write" ? t("collabs.write") : t("collabs.read")}
                  </Badge>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 shrink-0 text-destructive hover:text-destructive"
                    disabled={busy}
                    onClick={() => remove(c)}
                    title={t("collabs.remove")}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}

          <div className="grid gap-2">
            <Label htmlFor="collab-username">{t("collabs.usernameLabel")}</Label>
            <div className={cn("flex flex-col gap-2 sm:flex-row")}>
              <Input
                id="collab-username"
                placeholder={t("collabs.usernamePlaceholder")}
                autoCapitalize="none"
                autoCorrect="off"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    add();
                  }
                }}
              />
              <select
                aria-label={t("collabs.permission")}
                value={permission}
                onChange={(e) => setPermission(e.target.value as "read" | "write")}
                className="h-10 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-32"
              >
                <option value="write">{t("collabs.write")}</option>
                <option value="read">{t("collabs.read")}</option>
              </select>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={add} disabled={busy || !username.trim()}>
            <Plus className="h-4 w-4" />
            {t("collabs.add")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
