import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { KeyRound, Trash2 } from "lucide-react";
import { api, type DeployKey } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { RelativeTime } from "@/components/relative-time";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 仓库部署密钥（deploy key）：绑定单仓库的 SSH 公钥，只读或读写（仅 owner）。 */
export function DeployKeysCard({ owner, name }: { owner: string; name: string }) {
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
  const [keys, setKeys] = useState<DeployKey[]>([]);
  const [title, setTitle] = useState("");
  const [pub, setPub] = useState("");
  const [readOnly, setReadOnly] = useState(true);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    try {
      setKeys((await api.listDeployKeys(owner, name)) ?? []);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoaded(true);
    }
  }, [owner, name, to]);

  useEffect(() => {
    void load();
  }, [load]);

  const add = async () => {
    if (!title.trim() || !pub.trim()) return;
    setBusy(true);
    try {
      const key = await api.createDeployKey(owner, name, {
        title: title.trim(),
        key: pub.trim(),
        read_only: readOnly,
      });
      setKeys((ks) => [key, ...ks]);
      setTitle("");
      setPub("");
      setReadOnly(true);
      toast.success(t("repo.deployKeyAdded"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (id: number) => {
    try {
      await api.deleteDeployKey(owner, name, id);
      setKeys((ks) => ks.filter((k) => k.id !== id));
      toast.success(t("repo.deployKeyDeleted"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <KeyRound className="h-4 w-4" />
          {t("repo.deployKeysTitle")}
        </CardTitle>
        <CardDescription>{t("repo.deployKeysDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="deploy-key-title">{t("repo.deployKeyTitle")}</Label>
            <Input
              id="deploy-key-title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={t("repo.deployKeyTitlePlaceholder")}
              maxLength={100}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="deploy-key-public">{t("repo.deployKeyPublic")}</Label>
            <Textarea
              id="deploy-key-public"
              rows={3}
              value={pub}
              onChange={(e) => setPub(e.target.value)}
              placeholder="ssh-ed25519 AAAA…"
              className="font-mono text-xs"
            />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-input"
              checked={readOnly}
              onChange={(e) => setReadOnly(e.target.checked)}
            />
            {t("repo.deployKeyReadOnly")}
          </label>
          <div>
            <Button size="sm" disabled={busy || !title.trim() || !pub.trim()} onClick={add}>
              {t("repo.deployKeyAdd")}
            </Button>
          </div>
        </div>

        {loaded && keys.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("repo.deployKeyNone")}</p>
        ) : (
          <div className="divide-y divide-border rounded-md border">
            {keys.map((k) => (
              <div key={k.id} className="flex items-center gap-3 px-3 py-2">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">{k.name}</span>
                    <Badge variant="outline" className="shrink-0 text-xs">
                      {k.permission === "write"
                        ? t("repo.deployKeyWrite")
                        : t("repo.deployKeyRead")}
                    </Badge>
                  </div>
                  <div className="truncate font-mono text-xs text-muted-foreground">
                    {k.fingerprint}
                  </div>
                </div>
                <span className="hidden shrink-0 text-xs text-muted-foreground sm:block">
                  <RelativeTime iso={k.created_at} locale={locale} />
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-muted-foreground hover:text-destructive"
                  title={t("common.delete")}
                  aria-label={t("common.delete")}
                  onClick={() => remove(k.id)}
                >
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
