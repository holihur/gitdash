import { useEffect, useState } from "react";
import { toast } from "sonner";
import { AlertTriangle, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export function DangerZone() {
  const { t, to } = useI18n();
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [method, setMethod] = useState<string>("totp");
  const [mfaEnabled, setMfaEnabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const [sending, setSending] = useState(false);

  useEffect(() => {
    if (!open) return;
    api
      .mfaStatus()
      .then((s) => {
        setMfaEnabled(s.enabled);
        setMethod(s.method ?? "totp");
      })
      .catch(() => undefined);
  }, [open]);

  const sendCode = async () => {
    setSending(true);
    try {
      await api.mfaEmailSend();
      toast.success(t("profile.verificationSent"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSending(false);
    }
  };

  const remove = async () => {
    if (!password) return;
    setBusy(true);
    try {
      await api.deleteAccount(password, code.trim());
      toast.success(t("profile.deleteAccountSuccess"));
      // 账号已删除：强制刷新回登录页（会话 cookie 已清除）
      window.location.assign("/");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
      setBusy(false);
    }
  };

  return (
    <Card className="border-destructive/40">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base text-destructive">
          <AlertTriangle className="h-4 w-4" />
          {t("profile.dangerZone")}
        </CardTitle>
        <CardDescription>{t("profile.deleteAccountHint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Button
          variant="destructive"
          onClick={() => {
            setPassword("");
            setCode("");
            setOpen(true);
          }}
        >
          <Trash2 className="h-4 w-4" />
          {t("profile.deleteAccount")}
        </Button>
      </CardContent>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("profile.deleteAccountConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("profile.deleteAccountConfirmBody")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="delete-password">{t("profile.deleteAccountPassword")}</Label>
              <Input
                id="delete-password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {mfaEnabled && (
              <div className="grid gap-1.5">
                <Label htmlFor="delete-code">
                  {method === "email" ? t("profile.emailCodePlaceholder") : t("profile.authenticatorCode")}
                </Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="delete-code"
                    inputMode="numeric"
                    maxLength={6}
                    className="font-mono tracking-widest"
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
                  />
                  {method === "email" && (
                    <Button variant="outline" onClick={sendCode} disabled={sending || busy}>
                      {t("profile.deleteAccountSendCode")}
                    </Button>
                  )}
                </div>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setOpen(false)} disabled={busy}>
              {t("login.back")}
            </Button>
            <Button
              variant="destructive"
              onClick={remove}
              disabled={busy || !password || (mfaEnabled && code.length < 6)}
            >
              <Trash2 className="h-4 w-4" />
              {t("profile.deleteAccountButton")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

// ---- runners（自托管 CI agent） ----

