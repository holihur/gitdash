import { useEffect, useState } from "react";
import { toast } from "sonner";
import { BadgeCheck, UserRound } from "lucide-react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function EmailSection({
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

