import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Github, Gitlab, Link2, Link2Off, Loader2, Server } from "lucide-react";
import { api, type Connection } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

function ProviderIcon({ provider }: { provider: string }) {
  if (provider === "github") return <Github className="h-4 w-4" />;
  if (provider === "gitlab") return <Gitlab className="h-4 w-4" />;
  if (provider === "bitbucket") return <Server className="h-4 w-4" />;
  return <Server className="h-4 w-4" />;
}

/** 第三方账号绑定：连接 GitHub/GitLab/Gitea/Bitbucket，用于批量导入仓库。 */
export function ConnectionsSection() {
  const { t, to } = useI18n();
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    try {
      setConnections(await api.listConnections());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [to]);

  useEffect(() => {
    // 处理 OAuth 回调重定向带来的查询参数。
    const params = new URLSearchParams(window.location.search);
    const connected = params.get("connected");
    const err = params.get("connect_error");
    if (connected) toast.success(t("connections.connected", { provider: connected }));
    if (err) toast.error(err);
    if (connected || err) {
      params.delete("connected");
      params.delete("connect_error");
      const qs = params.toString();
      window.history.replaceState(null, "", window.location.pathname + (qs ? "?" + qs : ""));
    }
    void load();
    // 仅在挂载时读取一次查询参数
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const disconnect = async (provider: string) => {
    setBusy(provider);
    try {
      await api.disconnect(provider);
      toast.success(t("connections.disconnected", { provider }));
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy("");
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Link2 className="h-4 w-4" />
          {t("connections.title")}
        </CardTitle>
        <CardDescription>{t("connections.hint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-1">
        {loading ? (
          <p className="flex items-center justify-center gap-2 py-6 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> …
          </p>
        ) : (
          connections.map((c) => (
            <div key={c.provider} className="flex items-center gap-3 border-b py-3 last:border-0">
              <ProviderIcon provider={c.provider} />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-sm font-medium">
                  {c.label}
                  {c.connected && (
                    <Badge variant="secondary" className="gap-1">
                      <Link2 className="h-3 w-3" />
                      {c.login}
                    </Badge>
                  )}
                  {!c.enabled && <Badge variant="outline">{t("connections.notConfigured")}</Badge>}
                </div>
                {c.base_url && <p className="truncate text-xs text-muted-foreground">{c.base_url}</p>}
              </div>
              {c.connected ? (
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-2"
                  disabled={busy === c.provider}
                  onClick={() => disconnect(c.provider)}
                >
                  <Link2Off className="h-4 w-4" />
                  {t("connections.disconnect")}
                </Button>
              ) : (
                <Button
                  size="sm"
                  className="gap-2"
                  disabled={!c.enabled}
                  onClick={() => {
                    window.location.href = `/api/connections/${c.provider}/start`;
                  }}
                >
                  <Link2 className="h-4 w-4" />
                  {t("connections.connect")}
                </Button>
              )}
            </div>
          ))
        )}
      </CardContent>
    </Card>
  );
}
