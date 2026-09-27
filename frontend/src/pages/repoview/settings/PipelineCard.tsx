import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { buildRepoPath } from "@/lib/repo-url";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

/** 仓库设置里的流水线卡片：只读状态 + 跳转 Pipeline tab；开关本身只在 Pipeline
 * tab 上提供，避免两处重复的启用/禁用入口。 */
export function PipelineCard({ owner, name }: { owner: string; name: string }) {
  const { t } = useI18n();
  const [enabled, setEnabled] = useState<boolean | null>(null);

  useEffect(() => {
    api
      .getPipeline(owner, name)
      .then((p) => setEnabled(p.enabled))
      .catch(() => setEnabled(false));
  }, [owner, name]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("pipeline.title")}</CardTitle>
        <CardDescription>{t("pipeline.hint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={enabled ? "secondary" : "outline"}>
            {t(enabled ? "pipeline.statusOn" : "pipeline.statusOff")}
          </Badge>
          <Button size="sm" variant="outline" asChild>
            <Link to={buildRepoPath(owner, name, { tab: "pipeline" })}>
              {t("pipeline.manage")}
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
