import { useState } from "react";
import { Ban } from "lucide-react";
import { toast } from "sonner";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { adminReq } from "./api";

export function BanButton({
  path,
  banned,
  name,
  onChanged,
}: {
  path: string;
  banned: boolean;
  name: string;
  onChanged: () => void;
}) {
  const { t, to } = useI18n();
  const [busy, setBusy] = useState(false);

  const toggle = async () => {
    setBusy(true);
    try {
      await adminReq(path, { banned: !banned }, "POST");
      toast.success(t(banned ? "admin.unbannedMsg" : "admin.bannedMsg", { name }));
      onChanged();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Button
      variant="ghost"
      size="sm"
      className={`gap-1 ${banned ? "text-destructive hover:text-destructive" : ""}`}
      disabled={busy}
      onClick={toggle}
    >
      <Ban className="h-4 w-4" />
      {banned ? t("admin.unban") : t("admin.ban")}
    </Button>
  );
}

