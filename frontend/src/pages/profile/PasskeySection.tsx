import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Fingerprint, KeyRound, Plus, Trash2 } from "lucide-react";
import { api, type Passkey } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { createPasskey, passkeySupported } from "@/lib/webauthn";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import ConfirmDialog from "@/components/confirm-dialog";

export function PasskeySection() {
  const { t, to } = useI18n();
  const [passkeys, setPasskeys] = useState<Passkey[]>([]);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<Passkey | null>(null);

  const load = useCallback(async () => {
    try {
      const r = await api.passkeyList();
      setPasskeys(r.passkeys ?? []);
    } catch {
      // 旧服务端可能未实现；静默降级为空列表
      setPasskeys([]);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const supported = passkeySupported();

  const add = async () => {
    if (!supported) {
      toast.error(t("profile.passkeyUnsupported"));
      return;
    }
    setBusy(true);
    try {
      const begin = await api.passkeyRegisterBegin(name.trim());
      const credential = await createPasskey(begin);
      await api.passkeyRegisterFinish(begin.session_id, name.trim() || begin.name || "", credential);
      toast.success(t("profile.passkeyAdded"));
      setName("");
      await load();
    } catch (e) {
      // 用户取消浏览器弹窗（NotAllowedError）不应视为错误
      if ((e as { name?: string })?.name === "NotAllowedError") {
        toast.message(t("profile.passkeyCancelled"));
      } else {
        toast.error(apiErrorMsg(to, e));
      }
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!pendingDelete) return;
    setBusy(true);
    try {
      await api.passkeyDelete(pendingDelete.id);
      toast.success(t("profile.passkeyRemoved"));
      setPendingDelete(null);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Fingerprint className="h-4 w-4" />
          {t("profile.passkeys")}
        </CardTitle>
        <CardDescription>{t("profile.passkeysHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {passkeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("profile.passkeyEmpty")}</p>
        ) : (
          <ul className="divide-y rounded-md border">
            {passkeys.map((p) => (
              <li key={p.id} className="flex items-center justify-between gap-3 px-3 py-2">
                <div className="min-w-0">
                  <p className="flex items-center gap-2 truncate text-sm font-medium">
                    <KeyRound className="h-3.5 w-3.5 shrink-0" />
                    {p.name || t("profile.passkeyUnnamed")}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {p.backup_eligible ? t("profile.passkeySynced") : t("profile.passkeyDeviceBound")}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 shrink-0 text-muted-foreground hover:text-destructive"
                  onClick={() => setPendingDelete(p)}
                  title={t("common.delete")}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </li>
            ))}
          </ul>
        )}

        {supported && (
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <Input
              placeholder={t("profile.passkeyNamePlaceholder")}
              value={name}
              maxLength={100}
              onChange={(e) => setName(e.target.value)}
              className="sm:max-w-xs"
            />
            <Button onClick={add} disabled={busy}>
              <Plus className="h-4 w-4" />
              {t("profile.passkeyAdd")}
            </Button>
          </div>
        )}
      </CardContent>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title={t("profile.passkeyRemoveTitle")}
        description={t("profile.passkeyRemoveBody", {
          name: pendingDelete?.name || t("profile.passkeyUnnamed"),
        })}
        busy={busy}
        onConfirm={remove}
      />
    </Card>
  );
}
