import { useCallback, useEffect, useState } from "react";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type RepoEnvVar } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

export function RepoEnvVarsCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [vars, setVars] = useState<RepoEnvVar[]>([]);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setVars(await api.listRepoEnvVars(owner, name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const save = async () => {
    const k = key.trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(k)) {
      toast.error(t("repo.envInvalidKey"));
      return;
    }
    setBusy(true);
    try {
      setVars(await api.setRepoEnvVar(owner, name, k, value));
      setKey("");
      setValue("");
      toast.success(t("repo.envSaved", { key: k }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (k: string) => {
    try {
      await api.deleteRepoEnvVar(owner, name, k);
      setVars((v) => v.filter((x) => x.key !== k));
      toast.success(t("repo.envRemoved", { key: k }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <KeyRound className="h-4 w-4" />
          {t("repo.envTitle")}
        </CardTitle>
        <CardDescription>{t("repo.envDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.envKey")}
            <Input
              className="w-40 font-mono"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="KEY"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.envValue")}
            <Input
              className="w-64 font-mono"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="VALUE"
            />
          </label>
          <Button size="sm" className="gap-1 mb-0.5" disabled={busy || !key.trim()} onClick={save}>
            <Plus className="h-3.5 w-3.5" />
            {t("repo.envSave")}
          </Button>
        </div>

        {vars.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("repo.envEmpty")}</p>
        ) : (
          <div className="space-y-1.5">
            {vars.map((v) => (
              <div
                key={v.key}
                className="flex items-center justify-between gap-2 rounded-md border bg-muted/20 p-2 text-sm"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{v.key}</code>
                  <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                    {v.value}
                  </span>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive hover:text-destructive"
                  onClick={() => remove(v.key)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

