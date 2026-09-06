import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Toaster } from "sonner";
import {
  GitBranch,
  KeyRound,
  LogOut,
  Plus,
  Search,
  ShieldCheck,
  ShieldOff,
  Trash2,
  UserPlus,
} from "lucide-react";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

async function adminReq<T>(path: string, body?: unknown, method?: string): Promise<T> {
  const opts: RequestInit = {
    method: body === undefined ? method ?? "GET" : method ?? "POST",
    credentials: "same-origin",
    headers: {},
  };
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

async function adminList<T>(path: string): Promise<{ items: T; total: number }> {
  const res = await fetch(`/api/admin${path}`, { credentials: "same-origin" });
  if (res.status === 404) throw new ApiDisabledError();
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const data = await res.json();
      msg = (data as { error?: string })?.error ?? msg;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  const total = Number(res.headers.get("X-Total-Count") ?? "0");
  return { items: (await res.json()) as T, total: Number.isFinite(total) ? total : 0 };
}

type Translator = (key: string, vars?: Record<string, string | number>) => string | undefined;

/** 返回原始响应与解析后的错误体（含 code），供用户管理接口按错误码做 i18n。 */
async function adminUserReq(
  path: string,
  method: string,
  body?: unknown,
): Promise<
  | { ok: true; status: number }
  | { ok: false; status: number; code?: string; error?: string }
> {
  const opts: RequestInit = { method, credentials: "same-origin", headers: {} };
  if (body !== undefined) {
    opts.headers = { "Content-Type": "application/json" };
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`/api/admin${path}`, opts);
  let data: { error?: string; code?: string } | null = null;
  try {
    data = await res.json();
  } catch {
    /* ignore */
  }
  if (!res.ok) return { ok: false, status: res.status, code: data?.code, error: data?.error ?? res.statusText };
  return { ok: true, status: res.status };
}

function userActionError(
  to: Translator,
  r: { ok: false; status: number; code?: string; error?: string },
  fallbackKey: string,
): string {
  if (r.status === 409) return to("admin.userExists") ?? r.error ?? "conflict";
  if (r.code === "weak_password" || r.status === 400) return to("admin.weakPassword") ?? r.error ?? "bad request";
  if (r.status === 404) return to("admin.userNotFound") ?? r.error ?? "not found";
  return r.error ?? to(fallbackKey) ?? fallbackKey;
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
    <div className="mx-auto max-w-3xl space-y-6">
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
      <UsersSection />
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

interface AdminUser {
  id: number;
  username: string;
  email: string | null;
  created_at: string;
  mfa_enabled: boolean;
  notify_email: boolean;
}

const PAGE_SIZE = 20;

function UsersSection() {
  const [refresh, setRefresh] = useState(0);
  return (
    <div className="space-y-4">
      <CreateUserCard onCreated={() => setRefresh((n) => n + 1)} />
      <UsersCard refresh={refresh} />
    </div>
  );
}

function CreateUserCard({ onCreated }: { onCreated?: () => void }) {
  const { t, to } = useI18n();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMsg("");
    const r = await adminUserReq("/users", "POST", {
      username,
      password,
      email: email || undefined,
    });
    setBusy(false);
    if (!r.ok) {
      setMsg(userActionError(to, r, "admin.userCreated"));
      return;
    }
    toast.success(t("admin.userCreated", { name: username }));
    setUsername("");
    setEmail("");
    setPassword("");
    onCreated?.();
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <UserPlus className="h-4 w-4" />
          {t("admin.usersCreateTitle")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="grid gap-3 md:grid-cols-3">
          <div className="grid gap-2">
            <Label htmlFor="new-user-name">{t("admin.usersCreateUsername")}</Label>
            <Input id="new-user-name" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" required />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="new-user-email">{t("admin.usersCreateEmail")}</Label>
            <Input id="new-user-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="new-user-pass">{t("admin.usersCreatePassword")}</Label>
            <Input id="new-user-pass" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" minLength={8} required />
          </div>
          {msg && <p className="text-sm text-destructive md:col-span-3">{msg}</p>}
          <div className="md:col-span-3">
            <Button type="submit" disabled={busy || !username || password.length < 8}>
              <Plus className="h-4 w-4" />
              {t("admin.usersCreate")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function UsersCard({ refresh = 0 }: { refresh?: number }) {
  const { t, to } = useI18n();
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [query, setQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const load = useCallback(async (q: string, p: number) => {
    const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String((p - 1) * PAGE_SIZE) });
    if (q) params.set("q", q);
    try {
      const r = await adminList<AdminUser[]>(`/users?${params}`);
      setUsers(r.items);
      setTotal(r.total);
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);

  useEffect(() => {
    void load(activeQuery, page);
  }, [load, activeQuery, page, refresh]);

  const reload = () => {
    void load(activeQuery, page);
  };

  const search = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(1);
    setActiveQuery(query.trim());
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("admin.usersTitle")}</CardTitle>
        <CardDescription>{t("admin.usersHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <form onSubmit={search} className="flex gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              className="pl-8"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("admin.usersSearch")}
            />
          </div>
          <Button type="submit" variant="outline">{t("admin.usersSearchBtn")}</Button>
        </form>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.usersColUsername")}</TableHead>
              <TableHead>{t("admin.usersColEmail")}</TableHead>
              <TableHead>{t("admin.usersColCreated")}</TableHead>
              <TableHead>{t("admin.usersColMfa")}</TableHead>
              <TableHead className="text-right">{t("admin.usersColActions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users === null ? null : users.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="py-6 text-center text-muted-foreground">
                  {t("admin.usersEmpty")}
                </TableCell>
              </TableRow>
            ) : (
              users.map((u) => (
                <UserRow key={u.id} user={u} onChanged={reload} />
              ))
            )}
          </TableBody>
        </Table>
        <div className="flex items-center justify-end gap-2">
          <span className="text-sm text-muted-foreground">{t("admin.usersPageOf", { page, pages })}</span>
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            {t("admin.usersPrev")}
          </Button>
          <Button variant="outline" size="sm" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
            {t("admin.usersNext")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function UserRow({ user, onChanged }: { user: AdminUser; onChanged: () => void }) {
  const { t } = useI18n();
  const [resetOpen, setResetOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <TableRow>
      <TableCell className="font-medium">{user.username}</TableCell>
      <TableCell>{user.email || "—"}</TableCell>
      <TableCell>{new Date(user.created_at).toLocaleDateString()}</TableCell>
      <TableCell>
        <Badge variant={user.mfa_enabled ? "default" : "secondary"}>
          {user.mfa_enabled ? t("admin.on") : t("admin.off")}
        </Badge>
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-1">
          <Button variant="ghost" size="sm" className="gap-1" onClick={() => setResetOpen(true)}>
            <KeyRound className="h-4 w-4" />
            {t("admin.resetPassword")}
          </Button>
          <Button variant="ghost" size="sm" className="gap-1 text-destructive hover:text-destructive" onClick={() => setDeleteOpen(true)}>
            <Trash2 className="h-4 w-4" />
            {t("admin.deleteUser")}
          </Button>
        </div>
      </TableCell>
      <ResetPasswordDialog user={user.username} open={resetOpen} onOpenChange={setResetOpen} onDone={onChanged} />
      <DeleteUserDialog user={user.username} open={deleteOpen} onOpenChange={setDeleteOpen} onDone={onChanged} />
    </TableRow>
  );
}

function ResetPasswordDialog({
  user,
  open,
  onOpenChange,
  onDone,
}: {
  user: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDone: () => void;
}) {
  const { t, to } = useI18n();
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async () => {
    setBusy(true);
    setError("");
    const r = await adminUserReq(`/users/${encodeURIComponent(user)}/reset_password`, "POST", { password });
    setBusy(false);
    if (!r.ok) {
      setError(userActionError(to, r, "admin.passwordReset"));
      return;
    }
    toast.success(t("admin.passwordReset", { name: user }));
    setPassword("");
    onOpenChange(false);
    onDone();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("admin.resetPasswordTitle", { name: user })}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-2">
            <Label htmlFor={`reset-${user}`}>{t("admin.resetPasswordNew")}</Label>
            <Input
              id={`reset-${user}`}
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button onClick={submit} disabled={busy || password.length < 8}>
            {t("admin.resetPasswordConfirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteUserDialog({
  user,
  open,
  onOpenChange,
  onDone,
}: {
  user: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDone: () => void;
}) {
  const { t, to } = useI18n();
  const [busy, setBusy] = useState(false);

  const confirm = async () => {
    setBusy(true);
    const r = await adminUserReq(`/users/${encodeURIComponent(user)}`, "DELETE");
    setBusy(false);
    if (!r.ok) {
      toast.error(userActionError(to, r, "admin.userDeleted"));
    } else {
      toast.success(t("admin.userDeleted", { name: user }));
    }
    onOpenChange(false);
    onDone();
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("admin.deleteUserTitle", { name: user })}</AlertDialogTitle>
          <AlertDialogDescription>{t("admin.deleteUserHint")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy} />
          <AlertDialogAction onClick={confirm} disabled={busy}>
            {t("admin.deleteUser")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function toastError(to: (k: string, v?: Record<string, string | number>) => string | undefined, e: unknown) {
  toast.error(apiErrorMsg(to, e));
}
