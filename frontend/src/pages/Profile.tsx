import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { QRCodeSVG } from "qrcode.react";
import { BadgeCheck, Copy, KeyRound, ShieldCheck, ShieldOff, Trash2, UserRound } from "lucide-react";
import { api, type MFAEnroll, type MFAStatus, type GPGKey } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { copyText, formatDate } from "@/lib/utils";

interface Profile {
  username: string;
  created_at: string;
  mfa_enabled: boolean;
}

export default function ProfilePage() {
  const { t, to, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [profile, setProfile] = useState<Profile | null>(null);

  const loadProfile = useCallback(async () => {
    try {
      setProfile(await api.me());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [to]);

  useEffect(() => {
    loadProfile();
  }, [loadProfile]);

  if (!profile) return <p className="py-10 text-center text-sm text-muted-foreground">…</p>;

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("profile.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("profile.memberSince", { date: formatDate(profile.created_at, locale) })}
        </p>
      </div>

      <PasswordSection />
      <MFASection mfaEnabled={profile.mfa_enabled} onChanged={loadProfile} />
      <GPGKeySection />
    </div>
  );
}

function GPGKeySection() {
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

function PasswordSection() {
  const { t, to } = useI18n();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (next !== confirm) {
      toast.error(t("profile.passwordMismatch"));
      return;
    }
    setBusy(true);
    try {
      await api.changePassword(current, next);
      toast.success(t("profile.passwordChanged"));
      setCurrent("");
      setNext("");
      setConfirm("");
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
          <UserRound className="h-4 w-4" />
          {t("profile.changePassword")}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="grid gap-2">
          <Label htmlFor="pw-current">{t("profile.currentPassword")}</Label>
          <Input id="pw-current" type="password" value={current} onChange={(e) => setCurrent(e.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pw-new">{t("profile.newPassword")}</Label>
          <Input id="pw-new" type="password" value={next} onChange={(e) => setNext(e.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="pw-confirm">{t("profile.confirmPassword")}</Label>
          <Input id="pw-confirm" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </div>
        <div>
          <Button onClick={submit} disabled={busy || !current || !next || next.length < 8}>
            {t("profile.changePassword")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function MFASection({ mfaEnabled, onChanged }: { mfaEnabled: boolean; onChanged: () => void }) {
  const { t, to } = useI18n();
  const [status, setStatus] = useState<MFAStatus | null>(null);
  const [enroll, setEnroll] = useState<MFAEnroll | null>(null);
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
  const pendingSecret = status?.pending_secret;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {enabled ? <ShieldCheck className="h-4 w-4 text-green-600" /> : <ShieldOff className="h-4 w-4" />}
          {t("profile.mfa")}
        </CardTitle>
        <CardDescription>{enabled ? t("profile.mfaEnabledHint") : t("profile.mfaOffHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {!enabled && !enroll && !pendingSecret && (
          <Button onClick={startEnroll} disabled={busy}>
            <KeyRound className="h-4 w-4" />
            {t("profile.enable")}
          </Button>
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
            <p className="text-xs text-muted-foreground">{t("profile.disableRequires")}</p>
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
                placeholder={t("profile.authenticatorCode")}
                className="font-mono tracking-widest sm:w-40"
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              />
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
