import { useState } from "react";
import { toast } from "sonner";
import { CircleDot } from "lucide-react";
import { api, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** Issue 功能开关（仅 owner） */
export function IssuesFeatureCard({
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
  const [issuesBusy, setIssuesBusy] = useState(false);

  const toggleIssues = async () => {
    setIssuesBusy(true);
    try {
      const next = !(repo.has_issues ?? true);
      const r = await api.setRepoIssues(owner, name, next);
      setRepo({ ...repo, has_issues: r.has_issues });
      toast.success(t(r.has_issues ? "repo.issuesNowOn" : "repo.issuesNowOff"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setIssuesBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <CircleDot className="h-4 w-4" />
          {t("repo.issuesFeature")}
        </CardTitle>
        <CardDescription>{t("repo.issuesFeatureDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={repo.has_issues === false ? "outline" : "secondary"}>
            {repo.has_issues === false ? t("repo.issuesOff") : t("repo.issuesOn")}
          </Badge>
          <Button size="sm" variant="outline" disabled={issuesBusy} onClick={toggleIssues}>
            {repo.has_issues === false ? t("repo.enableIssues") : t("repo.disableIssues")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
