import { useState } from "react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { adminReq } from "./api";

export function PasswordCard() {
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

