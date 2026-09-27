import { useCallback, useEffect, useState } from "react";
import { Plus, Trash2, Webhook } from "lucide-react";
import { toast } from "sonner";
import { api, type IncomingWebhook } from "@/lib/api";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { copyText } from "@/lib/utils";
import { RelativeTime } from "@/components/relative-time";

export function IncomingWebhookCard({ owner, name }: { owner: string; name: string }) {
  const { t, to, lang } = useI18n();
  const [hooks, setHooks] = useState<IncomingWebhook[]>([]);
  const [path, setPath] = useState(`/api/hooks/incoming/${owner}/${name}`);
  const [hookName, setHookName] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api.listIncomingWebhooks(owner, name);
      setHooks(res.webhooks ?? []);
      setPath(res.path ?? `/api/hooks/incoming/${owner}/${name}`);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    setBusy(true);
    try {
      const res = await api.createIncomingWebhook(owner, name, hookName.trim());
      setPath(res.path);
      setToken(res.token);
      setHookName("");
      await load();
      toast.success(t("repo.incomingEnabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (hook: IncomingWebhook) => {
    setBusy(true);
    try {
      await api.deleteIncomingWebhook(owner, name, hook.id);
      if (token) setToken("");
      await load();
      toast.success(t("repo.incomingDisabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const toggle = async (hook: IncomingWebhook) => {
    setBusy(true);
    try {
      await api.setIncomingWebhookEnabled(owner, name, hook.id, !hook.enabled);
      setHooks((prev) => prev.map((h) => (h.id === hook.id ? { ...h, enabled: !hook.enabled } : h)));
      toast.success(t(hook.enabled ? "repo.incomingDisabledHook" : "repo.incomingEnabledHook"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const endpoint = `${typeof window !== "undefined" ? window.location.origin : ""}${path}`;
  const curl = `curl -X POST "${endpoint}" \\\n  -H "X-Gitdash-Token: ${token || "<TOKEN>"}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"title":"Issue title","body":"Issue body"}'`;

  const copy = (text: string, key: string) => {
    copyText(text)
      .then(() => toast.success(t(key)))
      .catch(() => toast.error(t("common.copyFailed")));
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Webhook className="h-4 w-4" />
          {t("repo.incomingWebhook")}
        </CardTitle>
        <CardDescription>{t("repo.incomingWebhookDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <Input
            className="sm:max-w-xs"
            maxLength={100}
            placeholder={t("repo.incomingNamePlaceholder")}
            value={hookName}
            onChange={(e) => setHookName(e.target.value)}
          />
          <Button size="sm" disabled={busy} onClick={create}>
            <Plus className="h-3.5 w-3.5" />
            {t("repo.incomingAdd")}
          </Button>
        </div>

        {hooks.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("repo.incomingEmpty")}</p>
        ) : (
          <ul className="divide-y rounded-md border">
            {hooks.map((hook) => (
              <li key={hook.id} className="flex items-center gap-3 px-3 py-2 text-sm">
                <span className="min-w-0 flex-1 truncate font-medium">{hook.name}</span>
                <Badge variant={hook.enabled ? "secondary" : "outline"} className="font-normal">
                  {hook.enabled ? t("repo.incomingOn") : t("repo.incomingOff")}
                </Badge>
                <span className="hidden whitespace-nowrap text-xs text-muted-foreground sm:inline">
                  {t("repo.incomingLastUsed")}{" "}
                  {hook.last_used_at ? (
                    <RelativeTime iso={hook.last_used_at} locale={dateLocale(lang)} />
                  ) : (
                    t("repo.incomingNever")
                  )}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 px-2 text-xs"
                  disabled={busy}
                  onClick={() => toggle(hook)}
                >
                  {hook.enabled ? t("repo.incomingPause") : t("repo.incomingResume")}
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-7 w-7 text-destructive hover:text-destructive"
                  disabled={busy}
                  title={t("repo.incomingDisable")}
                  aria-label={`${t("repo.incomingDisable")} ${hook.name}`}
                  onClick={() => remove(hook)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </li>
            ))}
          </ul>
        )}

        <div className="space-y-2 text-sm">
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t("repo.incomingEndpoint")}</span>
            <code className="min-w-0 flex-1 truncate rounded bg-muted px-1.5 py-0.5 text-xs">
              {path}
            </code>
            <Button size="sm" variant="ghost" onClick={() => copy(endpoint, "common.copied")}>
              {t("common.copy")}
            </Button>
          </div>
          {token ? (
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">{t("repo.incomingToken")}</span>
                <code className="min-w-0 flex-1 truncate rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                  {token}
                </code>
                <Button size="sm" variant="ghost" onClick={() => copy(token, "common.copied")}>
                  {t("common.copy")}
                </Button>
              </div>
              <p className="text-xs text-amber-600 dark:text-amber-400">
                {t("repo.incomingTokenOnce")}
              </p>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">{t("repo.incomingTokenHidden")}</p>
          )}
          <div className="space-y-1">
            <span className="text-xs text-muted-foreground">{t("repo.incomingExample")}</span>
            <pre className="overflow-x-auto rounded-md border bg-muted/40 p-2 text-xs">{curl}</pre>
            <Button size="sm" variant="outline" onClick={() => copy(curl, "common.copied")}>
              {t("common.copy")}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
