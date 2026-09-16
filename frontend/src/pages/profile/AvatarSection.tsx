import { useRef, useState } from "react";
import { toast } from "sonner";
import { UserRound } from "lucide-react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Avatar } from "@/components/avatar";

export function AvatarSection({ username }: { username: string }) {
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

