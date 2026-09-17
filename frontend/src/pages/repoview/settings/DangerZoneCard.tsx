import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import ConfirmDialog from "@/components/confirm-dialog";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 删除仓库（危险区） */
export function DangerZoneCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const navigate = useNavigate();
  const [deleteRepoOpen, setDeleteRepoOpen] = useState(false);
  const [deleteRepoBusy, setDeleteRepoBusy] = useState(false);

  const doDeleteRepo = async () => {
    setDeleteRepoBusy(true);
    try {
      await api.deleteRepo(owner, name);
      toast.success(t("repo.repoDeleted"));
      setDeleteRepoOpen(false);
      navigate("/repos");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setDeleteRepoBusy(false);
    }
  };

  return (
    <>
      <Card className="border-destructive/40">
        <CardHeader>
          <CardTitle className="text-base text-destructive">{t("repo.dangerZone")}</CardTitle>
          <CardDescription>{t("repo.deleteRepoDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            size="sm"
            variant="outline"
            className="text-destructive hover:text-destructive"
            onClick={() => setDeleteRepoOpen(true)}
          >
            <Trash2 className="h-3.5 w-3.5" />
            {t("repo.deleteRepo")}
          </Button>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={deleteRepoOpen}
        onOpenChange={setDeleteRepoOpen}
        title={t("repo.deleteRepo")}
        description={t("repo.deleteRepoConfirm", { name: `${owner}/${name}` })}
        onConfirm={doDeleteRepo}
        busy={deleteRepoBusy}
      />
    </>
  );
}
