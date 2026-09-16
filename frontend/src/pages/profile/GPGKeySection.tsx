import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { BadgeCheck, KeyRound, Trash2 } from "lucide-react";
import { api, type GPGKey } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";

export function GPGKeySection() {
  const { t, to } = useI18n();
  const [keys, setKeys] = useState<GPGKey[]>([]);
  const [armor, setArmor] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setKeys(await api.listGPGKeys());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [to]);

  useEffect(() => {
    load();
  }, [load]);

  const add = async () => {
    setBusy(true);
    try {
      await api.addGPGKey(armor.trim());
      toast.success(t("profile.gpgAdded"));
      setArmor("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (k: GPGKey) => {
    setBusy(true);
    try {
      await api.deleteGPGKey(k.id);
      toast.success(t("profile.gpgRemoved"));
      load();
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
          <BadgeCheck className="h-4 w-4" />
          {t("profile.gpg")}
        </CardTitle>
        <CardDescription>{t("profile.gpgHint")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {keys.length === 0 ? (
          <p className="rounded-lg border border-dashed py-6 text-center text-sm text-muted-foreground">
            {t("profile.gpgEmpty")}
          </p>
        ) : (
          <div className="divide-y divide-border rounded-lg border">
            {keys.map((k) => (
              <div key={k.id} className="flex items-center gap-3 px-3 py-2">
                <code className="min-w-0 flex-1 truncate font-mono text-xs">
                  {k.fingerprint.slice(0, 8)} … {k.fingerprint.slice(-8)}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 shrink-0 text-destructive hover:text-destructive"
                  disabled={busy}
                  onClick={() => remove(k)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
        <div className="grid gap-2">
          <Label htmlFor="gpg-armor">{t("profile.gpgArmor")}</Label>
          <textarea
            id="gpg-armor"
            rows={4}
            className="min-h-24 rounded-md border border-input bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("profile.gpgPlaceholder")}
            value={armor}
            onChange={(e) => setArmor(e.target.value)}
          />
        </div>
        <Button onClick={add} disabled={busy || !armor.trim()}>
          <KeyRound className="h-4 w-4" />
          {t("profile.gpgAdd")}
        </Button>
      </CardContent>
    </Card>
  );
}

