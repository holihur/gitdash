import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Cpu, Trash2 } from "lucide-react";
import { api, type Runner } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function RunnersSection() {
  const { t, to } = useI18n();
  const [runners, setRunners] = useState<Runner[] | null>(null);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setRunners(await api.listRunners());
    } catch {
      setRunners([]); // runner 功能未启用（无 redis）等情况
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const issue = async () => {
    setBusy(true);
    try {
      const res = await api.createRunnerToken("user");
      setToken(res.token);
      toast.success(t("runner.tokenIssued"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (name: string) => {
    try {
      await api.deleteRunner(name);
      toast.success(t("runner.deleted"));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base">
              <Cpu className="h-4 w-4" />
              {t("runner.title")}
            </CardTitle>
            <CardDescription className="mt-1">{t("runner.hint")}</CardDescription>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button asChild size="sm" variant="ghost">
              <Link to="/runners">{t("runnerGuide.title")}</Link>
            </Button>
            <Button size="sm" variant="outline" disabled={busy} onClick={issue}>
              {t("runner.issueToken")}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {token && (
          <div className="rounded-lg border bg-muted/40 p-3">
            <p className="mb-1 text-xs font-medium">{t("runner.tokenOnce")}</p>
            <code className="block break-all font-mono text-xs">{token}</code>
          </div>
        )}
        {runners === null ? null : runners.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">{t("runner.none")}</p>
        ) : (
          <div className="space-y-2">
            {runners.map((r) => (
              <div key={r.id} className="flex items-center justify-between rounded-lg border p-3">
                <div className="min-w-0">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <span
                      className={cn(
                        "inline-block h-2 w-2 rounded-full",
                        r.status === "online" ? "bg-emerald-500" : "bg-muted-foreground/40",
                      )}
                    />
                    {r.name}
                  </p>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">
                    {t(`runner.scope.${r.scope === "" ? "global" : r.scope.split(":")[0]}`)}
                    {r.mode === "reverse" && ` · ${t("runner.mode.reverse")}`}
                    {r.labels.length > 0 && ` · ${r.labels.join(", ")}`}
                  </p>
                </div>
                <Button size="icon" variant="ghost" onClick={() => remove(r.name)}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
