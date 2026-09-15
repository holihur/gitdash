import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { QRCodeSVG } from "qrcode.react";
import { BadgeCheck, Bot, Cpu, Copy, KeyRound, Loader2, Pencil, PlugZap, Plus, ShieldCheck, ShieldOff, Trash2, UserRound } from "lucide-react";
import { api, type MFAEnroll, type MFAStatus, type GPGKey, type Runner, type ByokKey } from "@/lib/api";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn, copyText, formatDate } from "@/lib/utils";
import { Avatar } from "@/components/avatar";

interface Profile {
  username: string;
  email?: string;
  created_at: string;
  mfa_enabled: boolean;
  email_verified?: boolean;
}

export default function ProfilePage() {
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
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

      <EmailSection
        email={profile.email ?? ""}
        verified={profile.email_verified ?? false}
        onChanged={loadProfile}
      />
      <AvatarSection username={profile.username} />
      <ByokSection />
      <PasswordSection />
      <MFASection mfaEnabled={profile.mfa_enabled} onChanged={loadProfile} />
      <GPGKeySection />
      <RunnersSection />
    </div>
  );
}

function EmailSection({
  email,
  verified,
  onChanged,
}: {
  email: string;
  verified: boolean;
  onChanged: () => void;
}) {
  const { t, to } = useI18n();
  const [value, setValue] = useState(email);
  const [busy, setBusy] = useState(false);

  useEffect(() => setValue(email), [email]);

  const resend = async () => {
    setBusy(true);
    try {
      await api.resendEmailVerification();
      toast.success(t("profile.verificationSent"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const save = async () => {
    setBusy(true);
    try {
      await api.updateProfile(value.trim());
      toast.success(t("profile.emailSaved"));
      onChanged();
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
          {t("profile.account")}
        </CardTitle>
        <CardDescription>{t("profile.emailHint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end sm:gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="profile-email">{t("profile.email")}</Label>
            <Input
              id="profile-email"
              type="email"
              placeholder="me@example.com"
              value={value}
              onChange={(e) => setValue(e.target.value)}
            />
          </div>
          <Button disabled={busy || value.trim() === email} onClick={save}>
            {t("common.save")}
          </Button>
        </div>
        {email && (
          <div className="mt-3 flex items-center gap-2 text-xs">
            {verified ? (
              <span className="flex items-center gap-1 text-green-600 dark:text-green-400">
                <BadgeCheck className="h-3.5 w-3.5" />
                {t("profile.emailVerifiedBadge")}
              </span>
            ) : (
              <>
                <span className="text-amber-600 dark:text-amber-400">
                  {t("profile.emailUnverified")}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-6 px-2 text-xs"
                  disabled={busy}
                  onClick={resend}
                >
                  {t("profile.resendVerification")}
                </Button>
              </>
            )}
          </div>
        )}
      </CardContent>
    </Card>
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

function AvatarSection({ username }: { username: string }) {
  const { t, to } = useI18n();
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const upload = async (file: File) => {
    setBusy(true);
    try {
      await api.uploadAvatar(file);
      setVersion(Date.now());
      toast.success(t("profile.avatarUpdated"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await api.deleteAvatar();
      setVersion(Date.now());
      toast.success(t("profile.avatarRemoved"));
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
          {t("profile.avatar")}
        </CardTitle>
        <CardDescription>{t("profile.avatarHint")}</CardDescription>
      </CardHeader>
      <CardContent className="flex items-center gap-4">
        <Avatar username={username} version={version} size={72} className="border" />
        <div className="flex flex-col gap-2">
          <input
            ref={inputRef}
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) upload(f);
              e.target.value = "";
            }}
          />
          <Button size="sm" variant="outline" disabled={busy} onClick={() => inputRef.current?.click()}>
            {t("profile.avatarUpload")}
          </Button>
          <Button size="sm" variant="ghost" disabled={busy} onClick={remove}>
            {t("profile.avatarRemove")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

const PROVIDERS = [
  { value: "anthropic", keyRequired: true, baseUrl: "https://api.anthropic.com", model: "claude-sonnet-4-5" },
  { value: "compatible", keyRequired: true, baseUrl: "", model: "" },
  { value: "ollama", keyRequired: false, baseUrl: "http://127.0.0.1:11434", model: "" },
] as const;

type ByokProvider = (typeof PROVIDERS)[number]["value"];

function providerLabel(t: (k: string) => string, provider: string): string {
  const key = `byok.provider.${provider}`;
  const label = t(key);
  return label === key ? provider : label;
}

function ByokSection() {
  const { t, to } = useI18n();
  const [keys, setKeys] = useState<ByokKey[] | null>(null);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<ByokKey | null>(null);
  const [name, setName] = useState("");
  const [provider, setProvider] = useState("anthropic");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [model, setModel] = useState("");
  const [busy, setBusy] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; msg: string } | null>(null);

  const load = useCallback(async () => {
    try {
      setKeys(await api.listByok());
    } catch {
      setKeys([]);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openAdd = () => {
    setEditing(null);
    setName("");
    setProvider("anthropic");
    setApiKey("");
    setBaseUrl("");
    setModel("");
    setTestResult(null);
    setOpen(true);
  };

  const openEdit = (k: ByokKey) => {
    setEditing(k);
    setName(k.name);
    setProvider(k.provider || "anthropic");
    setApiKey("");
    setBaseUrl(k.base_url ?? "");
    setModel(k.model ?? "");
    setTestResult(null);
    setOpen(true);
  };

  const specOf = (p: string) => PROVIDERS.find((x) => x.value === p) ?? PROVIDERS[0];
  const spec = specOf(provider);

  const changeProvider = (p: string) => {
    setProvider(p);
    setTestResult(null);
    const next = specOf(p);
    if (!baseUrl.trim()) setBaseUrl(next.baseUrl);
    if (!model.trim()) setModel(next.model);
  };

  const save = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      const body = {
        name: name.trim(),
        provider,
        api_key: apiKey.trim(),
        base_url: baseUrl.trim(),
        model: model.trim(),
      };
      if (editing) {
        await api.updateByok(editing.id, body);
      } else {
        await api.createByok(body);
      }
      toast.success(t("byok.saved"));
      setOpen(false);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await api.testByok({
        id: editing?.id,
        provider,
        api_key: apiKey.trim(),
        base_url: baseUrl.trim(),
        model: model.trim(),
      });
      setTestResult(res.ok ? { ok: true, msg: t("byok.testOk") } : { ok: false, msg: res.error || t("byok.testFail") });
    } catch (e) {
      setTestResult({ ok: false, msg: apiErrorMsg(to, e) });
    } finally {
      setTesting(false);
    }
  };

  const remove = async (k: ByokKey) => {
    try {
      await api.deleteByok(k.id);
      toast.success(t("byok.deleted"));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base">
              <Bot className="h-4 w-4" />
              {t("byok.title")}
            </CardTitle>
            <CardDescription className="mt-1">{t("byok.hint")}</CardDescription>
          </div>
          <Button size="sm" variant="outline" onClick={openAdd}>
            <Plus className="h-4 w-4" />
            {t("byok.add")}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {keys === null ? null : keys.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">{t("byok.empty")}</p>
        ) : (
          keys.map((k) => (
            <div key={k.id} className="flex items-center justify-between rounded-lg border p-3">
              <div className="min-w-0">
                <p className="flex items-center gap-2 text-sm font-medium">
                  {k.name}
                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    {providerLabel(t, k.provider)}
                  </span>
                </p>
                <p className="mt-0.5 truncate text-xs text-muted-foreground">
                  {k.model || "claude"}
                  {k.key_set
                    ? ` · ${t("byok.keySet")}`
                    : ` · ${t("byok.keyMissing")}`}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button size="icon" variant="ghost" onClick={() => openEdit(k)}>
                  <Pencil className="h-4 w-4" />
                </Button>
                <Button size="icon" variant="ghost" onClick={() => remove(k)}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))
        )}
      </CardContent>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t("byok.edit") : t("byok.add")}</DialogTitle>
            <DialogDescription>{t("byok.hint")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="byok-name">{t("byok.name")}</Label>
              <Input id="byok-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("byok.namePlaceholder")} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-provider">{t("byok.providerLabel")}</Label>
              <select
                id="byok-provider"
                className="rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                value={provider}
                onChange={(e) => changeProvider(e.target.value as ByokProvider)}
              >
                {PROVIDERS.map((p) => (
                  <option key={p.value} value={p.value}>
                    {providerLabel(t, p.value)}
                  </option>
                ))}
              </select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-key">
                {t("byok.apiKey")}
                {!spec.keyRequired && <span className="ml-1 text-xs text-muted-foreground">{t("byok.optional")}</span>}
              </Label>
              <Input id="byok-key" type="password" value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder={editing ? t("byok.apiKeyKeep") : t("byok.apiKeyPlaceholder")} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-base">{t("byok.baseUrl")}</Label>
              <Input id="byok-base" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder={spec.baseUrl || "https://your-gateway.example.com"} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-model">{t("byok.model")}</Label>
              <Input id="byok-model" value={model} onChange={(e) => setModel(e.target.value)} placeholder={spec.model || t("byok.modelPlaceholder")} />
            </div>
            {provider !== "anthropic" && (
              <p className="text-xs text-muted-foreground">{t("byok.compatibleHint")}</p>
            )}
            {testResult && (
              <p className={cn("text-xs", testResult.ok ? "text-green-600" : "text-destructive")}>
                {testResult.msg}
              </p>
            )}
          </div>
          <DialogFooter className="sm:justify-between">
            <Button variant="outline" onClick={test} disabled={testing || busy}>
              {testing ? <Loader2 className="h-4 w-4 animate-spin" /> : <PlugZap className="h-4 w-4" />}
              {t("byok.test")}
            </Button>
            <div className="flex items-center gap-2">
              <Button variant="ghost" onClick={() => setOpen(false)} disabled={busy}>
                {t("login.back")}
              </Button>
              <Button onClick={save} disabled={busy || !name.trim() || (!editing && spec.keyRequired && !apiKey.trim())}>
                {t("common.save")}
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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

// ---- runners（自托管 CI agent） ----

function RunnersSection() {
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
