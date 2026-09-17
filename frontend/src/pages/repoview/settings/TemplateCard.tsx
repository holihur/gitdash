import { useState } from "react";
import { toast } from "sonner";
import { BookTemplate } from "lucide-react";
import { api, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 模板仓库开关（仅 owner） */
export function TemplateCard({
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
  const [templateBusy, setTemplateBusy] = useState(false);

  const toggleTemplate = async () => {
    setTemplateBusy(true);
    try {
      const r = await api.setRepoTemplate(owner, name, !repo.is_template);
      setRepo({ ...repo, is_template: r.is_template });
      toast.success(t(r.is_template ? "repo.templateNowOn" : "repo.templateNowOff"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setTemplateBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <BookTemplate className="h-4 w-4" />
          {t("repo.templateRepo")}
        </CardTitle>
        <CardDescription>{t("repo.templateRepoDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={repo.is_template ? "secondary" : "outline"}>
            {repo.is_template ? t("repo.templateOn") : t("repo.templateOff")}
          </Badge>
          <Button size="sm" variant="outline" disabled={templateBusy} onClick={toggleTemplate}>
            {repo.is_template ? t("repo.makeNotTemplate") : t("repo.makeTemplate")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
