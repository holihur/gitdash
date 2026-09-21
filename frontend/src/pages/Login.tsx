import { useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import { GitBranch, Github, KeyRound, Mail, ShieldCheck } from "lucide-react";
import { api } from "@/lib/api";
import { useDocsUrl } from "@/lib/docs";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { takeReturnPath } from "@/lib/auth-expiry";
import { ThemeToggle, LangToggle } from "@/components/header-controls";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CredentialsFields, GoogleIcon, PasswordStrengthMeter } from "./login/parts";

interface Props {
  onAuthed: (username: string) => void;
}

export default function Login({ onAuthed }: Props) {
  const { t, to } = useI18n();
  const docs = useDocsUrl();
  const nav = useNavigate();
  const [searchParams] = useSearchParams();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  // 找回密码 / 重置密码视图（重置链接带 ?reset_password=token）
  const [resetToken] = useState(() => searchParams.get("reset_password") ?? "");
  const [view, setView] = useState<"auth" | "forgot" | "reset">(
    (searchParams.get("reset_password") ?? "") ? "reset" : "auth",
  );
  const [resetEnabled, setResetEnabled] = useState(false);
  const [email, setEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  // MFA 二次验证阶段
  const [mfaToken, setMfaToken] = useState("");
  const [mfaMethod, setMfaMethod] = useState<"totp" | "email">("totp");
  const [code, setCode] = useState("");
  const [githubEnabled, setGithubEnabled] = useState(false);
  const [googleEnabled, setGoogleEnabled] = useState(false);
  const [oidc, setOidc] = useState<{ enabled: boolean; name: string }>({ enabled: false, name: "OIDC" });
  const [version, setVersion] = useState("");
  const [providersError, setProvidersError] = useState(false);
  // 管理端可关闭的登录方式（缺省开启，兼容旧服务端）
  const [passwordEnabled, setPasswordEnabled] = useState(true);
  const [registerEnabled, setRegisterEnabled] = useState(true);
  const [swaggerEnabled, setSwaggerEnabled] = useState(true);

  const loadProviders = useCallback(() => {
    setProvidersError(false);
    api
      .authProviders()
      .then((d) => {
        setGithubEnabled(Boolean(d?.github?.enabled));
        setGoogleEnabled(Boolean(d?.google?.enabled));
        setOidc({ enabled: Boolean(d?.oidc?.enabled), name: d?.oidc?.name || "OIDC" });
        setResetEnabled(Boolean(d?.password_reset?.enabled));
        setPasswordEnabled(d?.password?.enabled !== false);
        setRegisterEnabled(d?.register?.enabled !== false);
        setSwaggerEnabled(d?.swagger?.enabled !== false);
      })
      .catch(() => {
        // 不再静默吞掉：接口异常时给出可见提示与重试入口
        setProvidersError(true);
      });
  }, []);

  useEffect(() => {
    api
      .version()
      .then((d) => {
        if (d?.version) setVersion(d.version);
      })
      .catch(() => undefined);
    loadProviders();
  }, [loadProviders]);

  const finish = (r: { token?: string; username?: string }) => {
    if (!r.token || !r.username) return;
    onAuthed(r.username);
    toast.success(t("login.welcomeBack", { name: r.username }));
    // 回跳优先级：URL ?redirect= > 401 暂存路径 > 首页
    nav(takeReturnPath(searchParams.get("redirect")), { replace: true });
  };

  const submit = async (mode: "login" | "register") => {
    if (!username.trim() || !password) {
      toast.error(t("login.missing"));
      return;
    }
    setBusy(true);
    try {
      if (mode === "login") {
        const r = await api.login(username.trim(), password);
        if (r.mfa_required && r.mfa_token) {
          setMfaToken(r.mfa_token);
          setMfaMethod(r.mfa_method === "email" ? "email" : "totp");
          setCode("");
          return; // 进入第二步
        }
        finish(r);
      } else {
        const r = await api.register(username.trim(), password);
        finish(r);
      }
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const submitCode = async () => {
    if (!code.trim()) return;
    setBusy(true);
    try {
      const r = await api.mfaVerify(mfaToken, code.trim());
      finish(r);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const resendCode = async () => {
    setBusy(true);
    try {
      await api.mfaEmailResend(mfaToken);
      toast.success(t("login.mfaCodeResent"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const clearResetParam = () => {
    const params = new URLSearchParams(window.location.search);
    params.delete("reset_password");
    const qs = params.toString();
    window.history.replaceState(null, "", window.location.pathname + (qs ? "?" + qs : ""));
  };

  const submitForgot = async () => {
    if (!email.trim()) {
      toast.error(t("login.forgotMissingEmail"));
      return;
    }
    setBusy(true);
    try {
      await api.forgotPassword(email.trim());
      toast.success(t("login.forgotSent"));
      setView("auth");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const submitReset = async () => {
    if (newPassword !== confirmPassword) {
      toast.error(t("login.resetMismatch"));
      return;
    }
    setBusy(true);
    try {
      await api.resetPassword(resetToken, newPassword);
      toast.success(t("login.resetDone"));
      clearResetParam();
      setNewPassword("");
      setConfirmPassword("");
      setView("auth");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  if (mfaToken) {
    return (
      <div className="flex min-h-screen flex-col">
        <div className="flex items-center justify-end gap-1 px-3 py-2 sm:px-6 sm:py-3">
          <ThemeToggle />
          <LangToggle />
        </div>
        <div className="flex flex-1 items-center justify-center px-4 pb-8">
          <Card className="w-full max-w-sm">
            <CardHeader className="items-center text-center">
              <div className="mx-auto mb-1 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <ShieldCheck className="h-6 w-6" />
              </div>
              <CardTitle>{t("login.mfaTitle")}</CardTitle>
              <CardDescription>
                {mfaMethod === "email" ? t("login.mfaSubtitleEmail") : t("login.mfaSubtitle")}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  submitCode();
                }}
              >
                <div className="grid gap-2">
                  <Label htmlFor="mfa-code">
                    {mfaMethod === "email" ? t("login.emailCode") : t("login.authenticatorCode")}
                  </Label>
                  <Input
                    id="mfa-code"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    placeholder="000000"
                    className="text-center text-lg tracking-widest"
                    value={code}
                    onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
                  />
                </div>
                <Button type="submit" className="mt-4 w-full" disabled={busy || code.length < 6}>
                  {t("login.verify")}
                </Button>
              </form>
              {mfaMethod === "email" && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full"
                  disabled={busy}
                  onClick={resendCode}
                >
                  {t("login.resendCode")}
                </Button>
              )}
              <Button
                variant="ghost"
                size="sm"
                className="w-full"
                onClick={() => setMfaToken("")}
              >
                {t("login.back")}
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  if (view === "forgot") {
    return (
      <div className="flex min-h-screen flex-col">
        <div className="flex items-center justify-end gap-1 px-3 py-2 sm:px-6 sm:py-3">
          <ThemeToggle />
          <LangToggle />
        </div>
        <div className="flex flex-1 items-center justify-center px-4 pb-8">
          <Card className="w-full max-w-sm">
            <CardHeader className="items-center text-center">
              <div className="mx-auto mb-1 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <Mail className="h-6 w-6" />
              </div>
              <CardTitle>{t("login.forgotTitle")}</CardTitle>
              <CardDescription>{t("login.forgotSubtitle")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  submitForgot();
                }}
              >
                <div className="grid gap-2">
                  <Label htmlFor="forgot-email">{t("login.email")}</Label>
                  <Input
                    id="forgot-email"
                    type="email"
                    autoComplete="email"
                    placeholder="you@example.com"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                  />
                </div>
                <Button type="submit" className="mt-4 w-full" disabled={busy}>
                  {t("login.forgotSend")}
                </Button>
              </form>
              <Button variant="ghost" size="sm" className="w-full" onClick={() => setView("auth")}>
                {t("login.back")}
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  if (view === "reset") {
    return (
      <div className="flex min-h-screen flex-col">
        <div className="flex items-center justify-end gap-1 px-3 py-2 sm:px-6 sm:py-3">
          <ThemeToggle />
          <LangToggle />
        </div>
        <div className="flex flex-1 items-center justify-center px-4 pb-8">
          <Card className="w-full max-w-sm">
            <CardHeader className="items-center text-center">
              <div className="mx-auto mb-1 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <KeyRound className="h-6 w-6" />
              </div>
              <CardTitle>{t("login.resetTitle")}</CardTitle>
              <CardDescription>{t("login.resetSubtitle")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  submitReset();
                }}
              >
                <div className="grid gap-2">
                  <Label htmlFor="reset-password">{t("login.newPassword")}</Label>
                  <Input
                    id="reset-password"
                    type="password"
                    autoComplete="new-password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                  />
                </div>
                <div className="mt-3 grid gap-2">
                  <Label htmlFor="reset-confirm">{t("login.confirmPassword")}</Label>
                  <Input
                    id="reset-confirm"
                    type="password"
                    autoComplete="new-password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                  />
                </div>
                <PasswordStrengthMeter password={newPassword} />
                <Button type="submit" className="mt-4 w-full" disabled={busy}>
                  {t("login.resetSubmit")}
                </Button>
              </form>
              <Button
                variant="ghost"
                size="sm"
                className="w-full"
                onClick={() => {
                  clearResetParam();
                  setView("auth");
                }}
              >
                {t("login.back")}
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col">
      <div className="flex items-center justify-end gap-2 px-3 py-2 sm:px-6 sm:py-3">
        {swaggerEnabled && (
          <a
            href="/api/swagger/"
            target="_blank"
            rel="noreferrer"
            className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
          >
            {t("login.apiDocs")}
          </a>
        )}
        <ThemeToggle />
        <LangToggle />
      </div>
      <div className="flex flex-1 items-center justify-center px-4 pb-8">
        <Card className="w-full max-w-sm">
          <CardHeader className="items-center text-center">
            <div className="mx-auto mb-1 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
              <GitBranch className="h-6 w-6" />
            </div>
            <CardTitle className="flex items-center justify-center gap-2">
              gitdash
              {version && (
                <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-mono font-normal text-muted-foreground">
                  {version}
                </span>
              )}
            </CardTitle>
            <CardDescription>{t("login.subtitle")}</CardDescription>
          </CardHeader>
          <CardContent>
            {passwordEnabled ? (
              <Tabs defaultValue="login">
                <TabsList className={registerEnabled ? "grid w-full grid-cols-2" : "grid w-full grid-cols-1"}>
                  <TabsTrigger value="login">{t("login.signIn")}</TabsTrigger>
                  {registerEnabled && <TabsTrigger value="register">{t("login.register")}</TabsTrigger>}
                </TabsList>
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    submit("login");
                  }}
                >
                  <TabsContent value="login" className="mt-4 space-y-4">
                    <CredentialsFields
                      username={username}
                      password={password}
                      setUsername={setUsername}
                      setPassword={setPassword}
                    />
                    <Button type="submit" className="w-full" disabled={busy}>
                      {t("login.signIn")}
                    </Button>
                    {resetEnabled && (
                      <button
                        type="button"
                        className="w-full text-center text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                        onClick={() => setView("forgot")}
                      >
                        {t("login.forgotLink")}
                      </button>
                    )}
                  </TabsContent>
                </form>
                {registerEnabled && (
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      submit("register");
                    }}
                  >
                    <TabsContent value="register" className="mt-4 space-y-4">
                      <CredentialsFields
                        username={username}
                        password={password}
                        setUsername={setUsername}
                        setPassword={setPassword}
                        passwordHint={t("login.passwordMin")}
                      />
                      <PasswordStrengthMeter password={password} />
                      <Button type="submit" className="w-full" disabled={busy}>
                        {t("login.registerAndSignIn")}
                      </Button>
                    </TabsContent>
                  </form>
                )}
              </Tabs>
            ) : (
              <p className="rounded-md border border-dashed px-3 py-3 text-center text-sm text-muted-foreground">
                {t("login.passwordDisabled")}
              </p>
            )}
            <p className="mt-4 text-center text-xs text-muted-foreground">{t("login.hint")}</p>
            {docs && (
              <p className="mt-2 text-center text-xs">
                <a
                  href={docs}
                  target="_blank"
                  rel="noreferrer"
                  className="text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                >
                  {t("login.docs")}
                </a>
              </p>
            )}
            {(githubEnabled || googleEnabled || oidc.enabled || providersError) && (
              <div className="mt-3 space-y-2">
                {githubEnabled && (
                  <a
                    href="/api/auth/github"
                    className="flex w-full items-center justify-center gap-2 rounded-md border border-input py-2 text-sm font-medium transition-colors hover:bg-accent"
                  >
                    <Github className="h-4 w-4" />
                    {t("login.signInWithGithub")}
                  </a>
                )}
                {googleEnabled && (
                  <a
                    href="/api/auth/google"
                    className="flex w-full items-center justify-center gap-2 rounded-md border border-input py-2 text-sm font-medium transition-colors hover:bg-accent"
                  >
                    <GoogleIcon className="h-4 w-4" />
                    {t("login.signInWithGoogle")}
                  </a>
                )}
                {oidc.enabled && (
                  <a
                    href="/api/auth/oidc/start"
                    className="flex w-full items-center justify-center gap-2 rounded-md border border-input py-2 text-sm font-medium transition-colors hover:bg-accent"
                  >
                    <ShieldCheck className="h-4 w-4" />
                    {t("login.signInWithOIDC", { name: oidc.name })}
                  </a>
                )}
                {providersError && (
                  <div className="flex items-center justify-between gap-2 rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground">
                    <span>{t("login.providersLoadFailed")}</span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-6 shrink-0 px-2"
                      onClick={loadProviders}
                    >
                      {t("common.retry")}
                    </Button>
                  </div>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

