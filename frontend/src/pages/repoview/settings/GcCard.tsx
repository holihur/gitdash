import { useState } from "react";
import { toast } from "sonner";
import { Recycle } from "lucide-react";
import { api, type Repo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import ConfirmDialog from "@/components/confirm-dialog";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { cn, formatSize } from "@/lib/utils";

/** 仓库 GC（仅 owner） */
export function GcCard({
  owner,
  name,
  repo,
  setRepo,
}: {
  owner: string;
  name: string;
  repo: Repo;
  setRepo: (repo: Repo) => void;
}) {
  const { t, to } = useI18n();
  const [gcOpen, setGcOpen] = useState(false);
  const [gcBusy, setGcBusy] = useState(false);

  const doGC = async () => {
    setGcBusy(true);
    try {
      const res = await api.gcRepo(owner, name);
      setRepo({ ...repo, size: res.after_bytes });
      if (res.freed_bytes > 0) toast.success(t("repo.gcDone", { freed: formatSize(res.freed_bytes) }));
      else toast.success(t("repo.gcDoneNoop"));
      setGcOpen(false);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setGcBusy(false);
    }
  };

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Recycle className="h-4 w-4" />
            {t("repo.gcTitle")}
          </CardTitle>
          <CardDescription>{t("repo.gcDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center gap-3">
            {typeof repo.size === "number" && (
              <span className="text-xs text-muted-foreground">
                {t("repo.gcCurrentSize", { size: formatSize(repo.size) })}
              </span>
            )}
            <Button
              size="sm"
              variant="outline"
              className="gap-1.5"
              disabled={gcBusy}
              onClick={() => setGcOpen(true)}
            >
              <Recycle className={cn("h-3.5 w-3.5", gcBusy && "animate-spin")} />
              {gcBusy ? t("repo.gcRunning") : t("repo.gcRun")}
            </Button>
          </div>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={gcOpen}
        onOpenChange={setGcOpen}
        title={t("repo.gcRun")}
        description={t("repo.gcConfirm", { name: `${owner}/${name}` })}
        onConfirm={doGC}
        busy={gcBusy}
      />
    </>
  );
}
