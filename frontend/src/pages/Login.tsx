import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { GitBranch, ShieldCheck } from "lucide-react";
import { api, setToken } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { ThemeToggle, LangToggle } from "@/components/header-controls";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface Props {
  onAuthed: (username: string) => void;
}

export default function Login({ onAuthed }: Props) {
  const { t, to } = useI18n();
  const nav = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  // MFA 二次验证阶段
  const [mfaToken, setMfaToken] = useState("");
  const [code, setCode] = useState("");

  const finish = (r: { token?: string; username?: string }) => {
    if (!r.token || !r.username) return;
    setToken(r.token);
    onAuthed(r.username);
    toast.success(t("login.welcomeBack", { name: r.username }));
    nav("/");
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
              <CardDescription>{t("login.mfaSubtitle")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  submitCode();
                }}
              >
                <div className="grid gap-2">
                  <Label htmlFor="mfa-code">{t("login.authenticatorCode")}</Label>
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
              <GitBranch className="h-6 w-6" />
            </div>
            <CardTitle>gitdash</CardTitle>
            <CardDescription>{t("login.subtitle")}</CardDescription>
          </CardHeader>
          <CardContent>
            <Tabs defaultValue="login">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="login">{t("login.signIn")}</TabsTrigger>
                <TabsTrigger value="register">{t("login.register")}</TabsTrigger>
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
                </TabsContent>
              </form>
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
                  <Button type="submit" className="w-full" disabled={busy}>
                    {t("login.registerAndSignIn")}
                  </Button>
                </TabsContent>
              </form>
            </Tabs>
            <p className="mt-4 text-center text-xs text-muted-foreground">{t("login.hint")}</p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function CredentialsFields(props: {
  username: string;
  password: string;
  setUsername: (v: string) => void;
  setPassword: (v: string) => void;
  passwordHint?: string;
}) {
  const { t } = useI18n();
  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label htmlFor="username">{t("login.username")}</Label>
        <Input
          id="username"
          placeholder={t("login.usernamePlaceholder")}
          autoComplete="username"
          value={props.username}
          onChange={(e) => props.setUsername(e.target.value)}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="password">
          {props.passwordHint
            ? t("login.passwordWithHint", { hint: props.passwordHint })
            : t("login.password")}
        </Label>
        <Input
          id="password"
          type="password"
          autoComplete={props.passwordHint ? "new-password" : "current-password"}
          value={props.password}
          onChange={(e) => props.setPassword(e.target.value)}
        />
      </div>
    </div>
  );
}
