import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Link2, Plus, Trash2 } from "lucide-react";
import { api, type Webhook } from "@/lib/api";
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
import { Label } from "@/components/ui/label";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  owner: string;
  repo: string;
}

export default function WebhooksDialog({ open, onOpenChange, owner, repo }: Props) {
  const { t, to } = useI18n();
  const [hooks, setHooks] = useState<Webhook[]>([]);
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setHooks(await api.listWebhooks(owner, repo));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, repo, to]);

  useEffect(() => {
    if (open) {
      setHooks([]);
      load();
    }
  }, [open, load]);

  const add = async () => {
    if (!url.trim()) return;
    setBusy(true);
    try {
      await api.createWebhook(owner, repo, url.trim());
      toast.success(t("webhooks.added"));
      setUrl("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (w: Webhook) => {
    setBusy(true);
    try {
      await api.deleteWebhook(owner, repo, w.id);
      toast.success(t("webhooks.removed"));
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
            {owner}/{repo} · {t("webhooks.manage")}
          </DialogTitle>
          <DialogDescription>{t("webhooks.emptyHint")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {hooks.length === 0 ? (
            <p className="flex items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-sm text-muted-foreground">
              <Link2 className="h-4 w-4" />
              {t("webhooks.empty")}
            </p>
          ) : (
            <div className="divide-y divide-border rounded-lg border">
              {hooks.map((w) => (
                <div key={w.id} className="flex items-center gap-3 px-3 py-2">
                  <code className="min-w-0 flex-1 truncate text-xs">{w.url}</code>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 shrink-0 text-destructive hover:text-destructive"
                    disabled={busy}
                    onClick={() => remove(w)}
                    title={t("webhooks.remove")}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}

          <div className="grid gap-2">
            <Label htmlFor="wh-url">{t("webhooks.urlLabel")}</Label>
            <Input
              id="wh-url"
              type="url"
              inputMode="url"
              placeholder={t("webhooks.urlPlaceholder")}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  add();
                }
              }}
            />
          </div>
        </div>

        <DialogFooter>
          <Button onClick={add} disabled={busy || !url.trim()}>
            <Plus className="h-4 w-4" />
            {t("webhooks.add")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
