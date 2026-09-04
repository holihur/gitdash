import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Toaster } from "sonner";
import { GitBranch, LogOut, ShieldCheck, ShieldOff } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { ThemeToggle, LangToggle } from "@/components/header-controls";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";

async function adminReq<T>(path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = { method: body === undefined ? "GET" : "POST", credentials: "same-origin", headers: {} };
  if (body !== undefined) {
    opts.headers = { "Content-Type": "application/json" };
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`/api/admin${path}`, opts);
  if (res.status === 404) throw new ApiDisabledError();
  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    /* ignore */
  }
  if (!res.ok) {
    const msg = (data as { error?: string })?.error ?? res.statusText;
    throw new Error(msg);
  }
  return data as T;
}

class ApiDisabledError extends Error {
  constructor() {
    super("admin_disabled");
    this.name = "ApiDisabledError";
  }
}

interface Settings {
  github_oauth_enabled: boolean;
  github_client_id: string;
  github_has_secret: boolean;
  oidc_enabled: boolean;
  oidc_name: string;
  oidc_issuer: string;
  oidc_client_id: string;
  oidc_has_secret: boolean;
}

export default function AdminApp() {
  const { t } = useI18n();
  const [me, setMe] = useState<string | null>(null);
  const [booted, setBooted] = useState(false);
  const [disabled, setDisabled] = useState(false);

  const boot = useCallback(async () => {
    try {
      const m = await adminReq<{ username: string }>("/me");
      setMe(m.username);
    } catch (e) {
      if (e instanceof ApiDisabledError) setDisabled(true);
      else setMe(null);
    } finally {
      setBooted(true);
    }
  }, []);

  useEffect(() => {
    boot();
  }, [boot]);

  if (!booted) return <p className="py-20 text-center text-muted-foreground">…</p>;

  if (disabled) {
    return (
      <ShellHeader>
        <Card className="mx-auto mt-16 max-w-md">
          <CardContent className="py-10 text-center">
            <GitBranch className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
            <p className="font-medium">{t("admin.disabledTitle")}</p>
            <p className="mt-2 text-sm text-muted-foreground">{t("admin.disabledHint")}</p>
          </CardContent>
        </Card>
      </ShellHeader>
    );
  }

  if (!me) {
    return (
      <ShellHeader>
        <LoginView onLogin={(u) => setMe(u)} />
      </ShellHeader>
    );
  }

  return <Dashboard user={me} onLogout={() => setMe(null)} />;
}

function ShellHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container flex h-14 items-center gap-2">
          <span className="flex items-center gap-2 text-lg font-bold">
            <GitBranch className="h-5 w-5" />
            gitdash Admin
          </span>
          <div className="ml-auto flex items-center gap-1">
            <ThemeToggle />
            <LangToggle />
          </div>
        </div>
      </header>
      <main className="container py-6">{children}</main>
      <Toaster richColors position="top-center" />
    </div>
  );
}

function LoginView({ onLogin }: { onLogin: (u: string) => void }) {
  const { t, to } = useI18n();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const r = await adminReq<{ username: string }>("/login", { username, password });
      onLogin(r.username);
    } catch (err) {
      if (err instanceof ApiDisabledError) setError(t("admin.disabledTitle"));
      else setError(apiErrorMsg(to, err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="mx-auto mt-16 max-w-sm">
      <CardHeader className="items-center text-center">
        <CardTitle>{t("admin.title")}</CardTitle>
        <CardDescription>{t("admin.signInHint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="adm-u">{t("login.username")}</Label>
            <Input id="adm-u" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="adm-p">{t("login.password")}</Label>
            <Input id="adm-p" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" disabled={busy || !username || !password}>
            {t("admin.signIn")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function Dashboard({ user, onLogout }: { user: string; onLogout: () => void }) {
  const { t, to } = useI18n();
  const [settings, setSettings] = useState<Settings | null>(null);

  const load = useCallback(async () => {
    try {
      setSettings(await adminReq<Settings>("/settings"));
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);

  useEffect(() => {
    void load();
  }, [load]);

  const logout = async () => {
    try {
      await adminReq("/logout", {});
    } catch {
      /* ignore */
    }
    onLogout();
  };

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("admin.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("admin.signedInAs", { user })}</p>
        </div>
        <Button variant="outline" size="sm" className="gap-2" onClick={logout}>
          <LogOut className="h-4 w-4" />
          {t("admin.signOut")}
        </Button>
      </div>

      <GithubSettings settings={settings} onChange={load} />
      <OidcSettings settings={settings} onChange={load} />
      <PasswordCard />
    </div>
  );
}

function GithubSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setEnabled(settings.github_oauth_enabled);
      setClientId(settings.github_client_id);
      setSecret("");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        github_oauth_enabled: enabled,
        github_client_id: clientId,
        github_client_secret: secret || undefined,
      });
      setMsg(t("admin.saved"));
      setSecret("");
      onChange();
    } catch (e) {
      setMsg(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {enabled ? <ShieldCheck className="h-4 w-4 text-green-600" /> : <ShieldOff className="h-4 w-4" />}
          {t("admin.githubTitle")}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t("admin.githubHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t("admin.githubEnable")}
        </label>
        <div className="grid gap-2">
          <Label htmlFor="gh-id">{t("admin.clientId")}</Label>
          <Input id="gh-id" value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="gh-secret">
            {t("admin.clientSecret")}
            {settings?.github_has_secret && `（${t("admin.secretSet")}）`}
          </Label>
          <Input id="gh-secret" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} autoComplete="new-password" />
        </div>
        <p className="text-xs text-muted-foreground">
          {t("admin.callbackLabel")}:{" "}
          <code className="break-all">{window.location.origin}/api/auth/github/callback</code>
        </p>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy || !clientId}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function OidcSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [name, setName] = useState("");
  const [issuer, setIssuer] = useState("");
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setEnabled(settings.oidc_enabled);
      setName(settings.oidc_name);
      setIssuer(settings.oidc_issuer);
      setClientId(settings.oidc_client_id);
      setSecret("");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        oidc_enabled: enabled,
        oidc_name: name,
        oidc_issuer: issuer,
        oidc_client_id: clientId,
        oidc_client_secret: secret || undefined,
      });
      setMsg(t("admin.saved"));
      setSecret("");
      onChange();
    } catch (e) {
      setMsg(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {enabled ? <ShieldCheck className="h-4 w-4 text-green-600" /> : <ShieldOff className="h-4 w-4" />}
          {t("admin.oidcTitle")}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t("admin.oidcHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t("admin.oidcEnable")}
        </label>
        <div className="grid gap-2">
          <Label htmlFor="oidc-name">{t("admin.oidcName")}</Label>
          <Input id="oidc-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="oidc-issuer">{t("admin.oidcIssuer")}</Label>
          <Input id="oidc-issuer" value={issuer} onChange={(e) => setIssuer(e.target.value)} placeholder="https://gitlab.com" autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="oidc-id">{t("admin.clientId")}</Label>
          <Input id="oidc-id" value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="oidc-secret">
            {t("admin.clientSecret")}
            {settings?.oidc_has_secret && `（${t("admin.secretSet")}）`}
          </Label>
          <Input id="oidc-secret" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} autoComplete="new-password" />
        </div>
        <p className="text-xs text-muted-foreground">
          {t("admin.callbackLabel")}:{" "}
          <code className="break-all">{window.location.origin}/api/auth/oidc/callback</code>
        </p>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy || !issuer || !clientId}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function PasswordCard() {
  const { t, to } = useI18n();
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const submit = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/password", { current_password: cur, new_password: next });
      setMsg(t("admin.passwordChanged"));
      setCur("");
      setNext("");
    } catch (e) {
      setMsg(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("admin.changePassword")}</CardTitle>
      </CardHeader>
      <CardContent className="grid max-w-sm gap-3">
        <Input type="password" placeholder={t("profile.currentPassword")} value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" />
        <Input type="password" placeholder={t("profile.newPassword")} value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" />
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={submit} disabled={busy || !cur || next.length < 8}>
            {t("admin.updatePassword")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function toastError(to: (k: string, v?: Record<string, string | number>) => string | undefined, e: unknown) {
  toast.error(apiErrorMsg(to, e));
}
