import { useCallback, useEffect, useState } from "react";
import { Lock, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type RepoSecret } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

const NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function SecretsCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [secrets, setSecrets] = useState<RepoSecret[]>([]);
  const [secretName, setSecretName] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setSecrets(await api.listRepoSecrets(owner, name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const save = async () => {
    const n = secretName.trim();
    if (!NAME_RE.test(n)) {
      toast.error(t("repo.secretInvalidName"));
      return;
    }
    setBusy(true);
    try {
      setSecrets(await api.setRepoSecret(owner, name, n, value));
      setSecretName("");
      setValue("");
      toast.success(t("repo.secretSaved", { name: n }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (n: string) => {
    try {
      await api.deleteRepoSecret(owner, name, n);
      setSecrets((s) => s.filter((x) => x.name !== n));
      toast.success(t("repo.secretRemoved", { name: n }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Lock className="h-4 w-4" />
          {t("repo.secretTitle")}
        </CardTitle>
        <CardDescription>{t("repo.secretDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">{t("repo.secretHint")}</p>
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.secretName")}
            <Input
              className="w-40 font-mono"
              value={secretName}
              onChange={(e) => setSecretName(e.target.value)}
              placeholder="MY_SECRET"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.secretValue")}
            <Input
              className="w-64 font-mono"
              type="password"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="••••••"
            />
          </label>
          <Button size="sm" className="mb-0.5 gap-1" disabled={busy || !secretName.trim()} onClick={save}>
            <Plus className="h-3.5 w-3.5" />
            {t("repo.secretSave")}
          </Button>
        </div>

        {secrets.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("repo.secretEmpty")}</p>
        ) : (
          <div className="space-y-1.5">
            {secrets.map((s) => (
              <div
                key={s.name}
                className="flex items-center justify-between gap-2 rounded-md border bg-muted/20 p-2 text-sm"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{s.name}</code>
                  <span className="font-mono text-xs text-muted-foreground">••••••••</span>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive hover:text-destructive"
                  onClick={() => remove(s.name)}
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
