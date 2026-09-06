import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { ChevronDown, ChevronRight, Link2, Plus, Trash2 } from "lucide-react";
import { api, type Webhook, type WebhookDelivery } from "@/lib/api";
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
  const [deliveries, setDeliveries] = useState<Record<number, WebhookDelivery[]>>({});
  const [expanded, setExpanded] = useState<number | null>(null);
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
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
      await api.createWebhook(owner, repo, url.trim(), secret.trim());
      toast.success(t("webhooks.added"));
      setUrl("");
      setSecret("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const toggle = async (w: Webhook) => {
    if (expanded === w.id) {
      setExpanded(null);
      return;
    }
    setExpanded(w.id);
    try {
      const list = await api.listWebhookDeliveries(owner, repo, w.id);
      setDeliveries((d) => ({ ...d, [w.id]: list }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
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
                <div key={w.id} className="px-3 py-2">
                  <div className="flex items-center gap-3">
                    <button
                      className="shrink-0 rounded p-0.5 hover:bg-muted"
                      onClick={() => toggle(w)}
                      title={t("webhooks.deliveries")}
                    >
                      {expanded === w.id ? (
                        <ChevronDown className="h-4 w-4" />
                      ) : (
                        <ChevronRight className="h-4 w-4" />
                      )}
                    </button>
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
                  {expanded === w.id && (
                    <div className="mt-2 space-y-1 border-l pl-3">
                      {(deliveries[w.id] ?? []).length === 0 ? (
                        <p className="py-1 text-xs text-muted-foreground">
                          {t("webhooks.noDeliveries")}
                        </p>
                      ) : (
                        deliveries[w.id].map((d) => (
                          <div key={d.id} className="flex items-center gap-2 text-xs">
                            <span
                              className={
                                d.status === "success"
                                  ? "font-medium text-green-600 dark:text-green-400"
                                  : d.status === "retry"
                                    ? "font-medium text-amber-600 dark:text-amber-400"
                                    : "font-medium text-destructive"
                              }
                            >
                              {t(`webhooks.status.${d.status}`)}
                            </span>
                            <span className="text-muted-foreground">{d.event}</span>
                            {d.code > 0 && <span className="text-muted-foreground">{d.code}</span>}
                            {d.attempts > 1 && (
                              <span className="text-muted-foreground">×{d.attempts}</span>
                            )}
                            {d.error && (
                              <span className="min-w-0 flex-1 truncate text-muted-foreground">
                                {d.error}
                              </span>
                            )}
                            <span className="ml-auto shrink-0 text-muted-foreground">
                              {new Date(d.created_at.endsWith("Z") ? d.created_at : d.created_at + "Z").toLocaleString()}
                            </span>
                          </div>
                        ))
                      )}
                    </div>
                  )}
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
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="wh-secret">{t("webhooks.secretLabel")}</Label>
            <Input
              id="wh-secret"
              type="password"
              autoComplete="off"
              placeholder={t("webhooks.secretPlaceholder")}
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("webhooks.secretHint")}</p>
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
