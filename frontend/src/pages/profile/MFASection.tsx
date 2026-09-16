import { useEffect, useState } from "react";
import { toast } from "sonner";
import { QRCodeSVG } from "qrcode.react";
import { Copy, KeyRound, ShieldCheck, ShieldOff } from "lucide-react";
import { api, type MFAEnroll, type MFAStatus } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { copyText } from "@/lib/utils";

export function MFASection({ mfaEnabled, onChanged }: { mfaEnabled: boolean; onChanged: () => void }) {
  const { t, to } = useI18n();
  const [status, setStatus] = useState<MFAStatus | null>(null);
  const [enroll, setEnroll] = useState<MFAEnroll | null>(null);
  const [emailEnrolling, setEmailEnrolling] = useState(false);
  const [code, setCode] = useState("");
  const [disablePw, setDisablePw] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .mfaStatus()
      .then((s) => setStatus(s))
      .catch(() => undefined);
  }, [mfaEnabled]);

  const startEnroll = async () => {
    setBusy(true);
    try {
      setEnroll(await api.mfaEnroll());
      setCode("");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const startEmailEnroll = async () => {
    setBusy(true);
    try {
      await api.mfaEmailEnroll();
      setEmailEnrolling(true);
      setCode("");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const activateEmail = async () => {
    setBusy(true);
    try {
      await api.mfaEmailActivate(code.trim());
      toast.success(t("profile.mfaEnabledToast"));
      setEmailEnrolling(false);
      setStatus(null);
      setCode("");
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const activate = async () => {
    setBusy(true);
    try {
      await api.mfaActivate(code.trim());
      toast.success(t("profile.mfaEnabledToast"));
      setEnroll(null);
      setStatus(null);
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const sendDisableCode = async () => {
    setBusy(true);
    try {
      await api.mfaEmailSend();
      toast.success(t("profile.verificationSent"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const disable = async () => {
    if (!disablePw) return;
    setBusy(true);
    try {
      await api.mfaDisable(disablePw, code || "");
      toast.success(t("profile.mfaDisabledToast"));
      setStatus(null);
      setDisablePw("");
      setCode("");
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const enabled = status?.enabled ?? mfaEnabled;
  const method = status?.method ?? "totp";
  const pendingSecret = status?.pending_secret;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {enabled ? <ShieldCheck className="h-4 w-4 text-green-600" /> : <ShieldOff className="h-4 w-4" />}
          {t("profile.mfa")}
        </CardTitle>
        <CardDescription>
          {enabled
            ? t(method === "email" ? "profile.mfaEnabledHintEmail" : "profile.mfaEnabledHint")
            : t("profile.mfaOffHint")}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {!enabled && !enroll && !pendingSecret && !emailEnrolling && (
          <div className="flex flex-col gap-2 sm:flex-row">
            <Button onClick={startEnroll} disabled={busy}>
              <KeyRound className="h-4 w-4" />
              {t("profile.enableTotp")}
            </Button>
            <Button variant="outline" onClick={startEmailEnroll} disabled={busy}>
              {t("profile.enableEmail")}
            </Button>
          </div>
        )}

        {emailEnrolling && (
          <>
            <p className="text-sm font-medium">{t("profile.emailSetupTitle")}</p>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Input
                inputMode="numeric"
                maxLength={6}
                placeholder={t("profile.emailCodePlaceholder")}
                className="font-mono tracking-widest sm:w-40"
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              />
              <Button onClick={activateEmail} disabled={busy || code.length < 6}>
                {t("profile.activate")}
              </Button>
              <Button variant="ghost" onClick={() => setEmailEnrolling(false)} disabled={busy}>
                {t("login.back")}
              </Button>
            </div>
          </>
        )}

        {!enabled && (enroll || pendingSecret) && (
          <>
            <p className="text-sm font-medium">{t("profile.setupTitle")}</p>
            <div className="flex flex-col items-center gap-3 sm:flex-row sm:items-start">
              <div className="rounded-lg border bg-white p-3">
                <QRCodeSVG value={enroll?.otpauth_url ?? status?.otpauth_url ?? ""} size={176} />
              </div>
              <div className="w-full min-w-0 space-y-2 sm:w-auto">
                <Label>{t("profile.secretLabel")}</Label>
                <div className="flex items-center gap-2 rounded-md border bg-muted/40 px-2 py-1.5">
                  <code className="min-w-0 flex-1 break-all font-mono text-xs">
                    {enroll?.secret ?? pendingSecret}
                  </code>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 shrink-0"
                    onClick={() => {
                      copyText(enroll?.secret ?? pendingSecret ?? "")
                        .then(() => toast.success(t("profile.secretCopied")))
                        .catch(() => toast.error(t("common.copyFailed")));
                    }}
                    title={t("profile.copySecret")}
                  >
                    <Copy className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            </div>
            <p className="text-sm font-medium">{t("profile.setupStep2")}</p>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Input
                inputMode="numeric"
                maxLength={6}
                placeholder={t("profile.authenticatorCode")}
                className="font-mono tracking-widest sm:w-40"
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              />
              <Button onClick={activate} disabled={busy || code.length < 6}>
                {t("profile.activate")}
              </Button>
              <Button variant="ghost" onClick={() => { setEnroll(null); setStatus(null); }} disabled={busy}>
                {t("login.back")}
              </Button>
            </div>
          </>
        )}

        {enabled && (
          <div className="flex flex-col gap-3">
            <p className="text-xs text-muted-foreground">
              {t(method === "email" ? "profile.disableRequiresEmail" : "profile.disableRequires")}
            </p>
            <div className="grid gap-2 sm:max-w-xs">
              <Input
                type="password"
                placeholder={t("profile.currentPassword")}
                value={disablePw}
                onChange={(e) => setDisablePw(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Input
                inputMode="numeric"
                maxLength={6}
                placeholder={
                  method === "email" ? t("profile.emailCodePlaceholder") : t("profile.authenticatorCode")
                }
                className="font-mono tracking-widest sm:w-40"
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              />
              {method === "email" && (
                <Button variant="outline" onClick={sendDisableCode} disabled={busy}>
                  {t("profile.sendCode")}
                </Button>
              )}
              <Button
                variant="destructive"
                onClick={disable}
                disabled={busy || !disablePw || code.length < 6}
              >
                <ShieldOff className="h-4 w-4" />
                {t("profile.disable")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ---- 账号注销（危险操作） ----

