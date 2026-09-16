import { useState } from "react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminReq, ApiDisabledError } from "./api";

export function LoginView({ onLogin }: { onLogin: (u: string) => void }) {
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

