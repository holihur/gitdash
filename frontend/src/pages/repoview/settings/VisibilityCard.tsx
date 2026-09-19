import { useState } from "react";
import { toast } from "sonner";
import { api, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 仓库可见性：private（成员/协作者）/ public（登录可见）/ anonymous（匿名只读）。 */
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
  const [busy, setBusy] = useState(false);

  const visibility = repo?.visibility || (repo?.private ? "private" : "public");

  const change = async (next: string) => {
    if (!repo) return;
    setBusy(true);
    try {
      const r = await api.setRepoVisibility(owner, name, next);
      setRepo({ ...repo, private: r.private, visibility: r.visibility });
      toast.success(t("repo.visibilityUpdated"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const label = (v: string) =>
    t(v === "private" ? "repo.privateRepo" : v === "anonymous" ? "repo.anonRepo" : "repo.publicRepo");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("repo.visibility")}</CardTitle>
        <CardDescription>{t("repo.visibilityDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={visibility === "private" ? "secondary" : "outline"}>{label(visibility)}</Badge>
          <select
            value={visibility}
            disabled={busy || !repo}
            onChange={(e) => void change(e.target.value)}
            aria-label={t("repo.visibility")}
            className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="private">{t("repo.privateRepo")}</option>
            <option value="public">{t("repo.publicRepo")}</option>
            <option value="anonymous">{t("repo.anonRepo")}</option>
          </select>
        </div>
      </CardContent>
    </Card>
  );
}
