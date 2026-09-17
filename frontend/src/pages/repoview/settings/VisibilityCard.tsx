import { useState } from "react";
import { toast } from "sonner";
import { api, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 仓库可见性切换（任意角色均可见） */
export function VisibilityCard({
  owner,
  name,
  repo,
  setRepo,
}: {
  owner: string;
  name: string;
  repo: Repo | null;
  setRepo: (repo: Repo) => void;
}) {
  const { t, to } = useI18n();
  const [visibilityBusy, setVisibilityBusy] = useState(false);

  const toggleVisibility = async () => {
    if (!repo) return;
    setVisibilityBusy(true);
    try {
      const r = await api.setRepoVisibility(owner, name, !repo.private);
      setRepo({ ...repo, private: r.private });
      toast.success(t(r.private ? "repo.visibilityNowPrivate" : "repo.visibilityNowPublic"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setVisibilityBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("repo.visibility")}</CardTitle>
        <CardDescription>{t("repo.visibilityDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={repo?.private ? "secondary" : "outline"}>
            {repo?.private ? t("repo.privateRepo") : t("repo.publicRepo")}
          </Badge>
          <Button
            size="sm"
            variant="outline"
            disabled={visibilityBusy || !repo}
            onClick={toggleVisibility}
          >
            {repo?.private ? t("repo.makePublic") : t("repo.makePrivate")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
