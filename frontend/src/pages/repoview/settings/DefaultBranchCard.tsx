import { useEffect, useState } from "react";
import { toast } from "sonner";
import { GitBranch } from "lucide-react";
import { api, type Branch, type Repo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 默认分支设置（仅 owner） */
export function DefaultBranchCard({
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
  const [branches, setBranches] = useState<Branch[]>([]);
  const [defaultBranch, setDefaultBranch] = useState("");
  const [branchBusy, setBranchBusy] = useState(false);

  useEffect(() => {
    api
      .branches(owner, name)
      .then(setBranches)
      .catch(() => setBranches([]));
  }, [owner, name]);

  useEffect(() => {
    setDefaultBranch(repo.default_branch || branches[0]?.name || "");
  }, [repo.default_branch, branches]);

  const saveDefaultBranch = async () => {
    if (!defaultBranch) return;
    setBranchBusy(true);
    try {
      const r = await api.setRepoDefaultBranch(owner, name, defaultBranch);
      setRepo({ ...repo, default_branch: r.default_branch });
      toast.success(t("repo.defaultBranchSaved", { branch: r.default_branch ?? defaultBranch }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBranchBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <GitBranch className="h-4 w-4" />
          {t("repo.defaultBranch")}
        </CardTitle>
        <CardDescription>{t("repo.defaultBranchDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="h-9 max-w-full rounded-md border bg-background px-2 text-sm"
            value={defaultBranch}
            disabled={branchBusy || branches.length === 0}
            onChange={(e) => setDefaultBranch(e.target.value)}
          >
            {branches.map((b) => (
              <option key={b.name} value={b.name}>
                {b.name}
              </option>
            ))}
          </select>
          <Button
            size="sm"
            disabled={branchBusy || !defaultBranch || defaultBranch === (repo.default_branch ?? "")}
            onClick={saveDefaultBranch}
          >
            {t("common.save")}
          </Button>
        </div>
        {branches.length === 0 && (
          <p className="mt-2 text-xs text-muted-foreground">{t("repo.defaultBranchEmpty")}</p>
        )}
      </CardContent>
    </Card>
  );
}
