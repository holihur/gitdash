import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { GitBranch } from "lucide-react";
import { api, setToken } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
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
  const { t } = useI18n();
  const nav = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (mode: "login" | "register") => {
    if (!username.trim() || !password) {
      toast.error(t("login.missing"));
      return;
    }
    setBusy(true);
    try {
      const r =
        mode === "login"
          ? await api.login(username.trim(), password)
          : await api.register(username.trim(), password);
      setToken(r.token);
      onAuthed(r.username);
      toast.success(
        mode === "login"
          ? t("login.welcomeBack", { name: r.username })
          : t("login.accountCreated", { name: r.username }),
      );
      nav("/");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

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
