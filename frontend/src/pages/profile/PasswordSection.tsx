import { useState } from "react";
import { toast } from "sonner";
import { UserRound } from "lucide-react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function PasswordSection() {
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

