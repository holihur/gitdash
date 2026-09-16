import { useCallback, useEffect, useState } from "react";
import { Trash2, Webhook } from "lucide-react";
import { toast } from "sonner";
import { api, type IncomingWebhook } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { copyText } from "@/lib/utils";

export function IncomingWebhookCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [status, setStatus] = useState<IncomingWebhook | null>(null);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setStatus(await api.getIncomingWebhook(owner, name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const enable = async () => {
    setBusy(true);
    try {
      const res = await api.setIncomingWebhook(owner, name);
      setStatus(res);
      setToken(res.token ?? "");
      toast.success(t(status?.enabled ? "repo.incomingRotated" : "repo.incomingEnabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const disable = async () => {
    setBusy(true);
    try {
      await api.deleteIncomingWebhook(owner, name);
      setStatus({ enabled: false });
      setToken("");
      toast.success(t("repo.incomingDisabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const path = status?.path ?? `/api/hooks/incoming/${owner}/${name}`;
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
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={status?.enabled ? "secondary" : "outline"}>
            {status?.enabled ? t("repo.incomingOn") : t("repo.incomingOff")}
          </Badge>
          <Button size="sm" variant="outline" disabled={busy || !status} onClick={enable}>
            {status?.enabled ? t("repo.incomingRotate") : t("repo.incomingEnable")}
          </Button>
          {status?.enabled && (
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              disabled={busy}
              onClick={disable}
            >
              <Trash2 className="h-3.5 w-3.5" />
              {t("repo.incomingDisable")}
            </Button>
          )}
        </div>
        {status?.enabled && (
          <div className="space-y-2 text-sm">
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">{t("repo.incomingEndpoint")}</span>
              <code className="min-w-0 flex-1 truncate rounded bg-muted px-1.5 py-0.5 text-xs">
                {path}
              </code>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => copy(endpoint, "common.copied")}
              >
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
              <pre className="overflow-x-auto rounded-md border bg-muted/40 p-2 text-xs">
                {curl}
              </pre>
              <Button size="sm" variant="outline" onClick={() => copy(curl, "common.copied")}>
                {t("common.copy")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

