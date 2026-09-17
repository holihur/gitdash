import { useEffect, useState } from "react";
import { toast } from "sonner";
import { PencilLine } from "lucide-react";
import { api, type Repo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 仓库描述（仅 owner） */
export function DescriptionCard({
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
  const [description, setDescription] = useState("");
  const [descBusy, setDescBusy] = useState(false);

  useEffect(() => {
    setDescription(repo.description ?? "");
  }, [repo.description]);

  const saveDescription = async () => {
    setDescBusy(true);
    try {
      const r = await api.setRepoDescription(owner, name, description.trim());
      setRepo({ ...repo, description: r.description });
      setDescription(r.description);
      toast.success(t("repo.descriptionSaved"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setDescBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <PencilLine className="h-4 w-4" />
          {t("repo.description")}
        </CardTitle>
        <CardDescription>{t("repo.descriptionDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex max-w-xl flex-wrap items-center gap-2">
          <Input
            value={description}
            maxLength={500}
            placeholder={t("repo.descriptionPlaceholder")}
            disabled={descBusy}
            className="min-w-0 flex-1"
            onChange={(e) => setDescription(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void saveDescription();
            }}
          />
          <Button
            size="sm"
            disabled={descBusy || description.trim() === (repo.description ?? "")}
            onClick={saveDescription}
          >
            {t("common.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
