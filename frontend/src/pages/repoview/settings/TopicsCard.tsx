import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Tag } from "lucide-react";
import { api, type Repo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 仓库 topics（仅 owner） */
export function TopicsCard({
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
  const [topicsInput, setTopicsInput] = useState("");
  const [topicsBusy, setTopicsBusy] = useState(false);

  useEffect(() => {
    setTopicsInput((repo.topics ?? []).join(", "));
  }, [repo.topics]);

  const saveTopics = async () => {
    setTopicsBusy(true);
    try {
      const topics = topicsInput
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      const r = await api.setRepoTopics(owner, name, topics);
      setRepo({ ...repo, topics: r.topics });
      setTopicsInput(r.topics.join(", "));
      toast.success(t("explore.topicsUpdated"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setTopicsBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Tag className="h-4 w-4" />
          {t("explore.editTopics")}
        </CardTitle>
        <CardDescription>{t("explore.topicsHint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex max-w-xl flex-wrap items-center gap-2">
          <Input
            value={topicsInput}
            placeholder={t("explore.topicsPlaceholder")}
            disabled={topicsBusy}
            className="min-w-0 flex-1"
            onChange={(e) => setTopicsInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void saveTopics();
            }}
          />
          <Button size="sm" disabled={topicsBusy} onClick={saveTopics}>
            {t("common.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
