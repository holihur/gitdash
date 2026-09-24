import { useEffect, useState } from "react";
import { toast } from "sonner";
import { ShieldCheck } from "lucide-react";
import { api, type Repo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { COLLAB_ROLES } from "@/lib/repo-role";

/** 组织成员在本仓库的默认角色覆盖（仅组织仓库、需 admin）。 */
export function MemberRoleCard({
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
  const [role, setRole] = useState(repo.member_role ?? "");
  const [orgDefault, setOrgDefault] = useState("write");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setRole(repo.member_role ?? "");
  }, [repo.member_role]);

  useEffect(() => {
    let alive = true;
    api
      .getOrgProfile(owner)
      .then((p) => {
        if (alive) setOrgDefault(p.default_member_role || "write");
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [owner]);

  const save = async () => {
    setBusy(true);
    try {
      const r = await api.setRepoMemberRole(owner, name, role);
      setRepo({ ...repo, member_role: r.member_role });
      toast.success(t("repo.memberRoleSaved"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" />
          {t("repo.memberRole")}
        </CardTitle>
        <CardDescription>{t("repo.memberRoleDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-2">
          <select
            aria-label={t("repo.memberRole")}
            className="h-9 rounded-md border border-input bg-background px-2 text-sm"
            value={role}
            disabled={busy}
            onChange={(e) => setRole(e.target.value)}
          >
            <option value="">
              {t("repo.memberRoleInherit", { role: t(`collabs.${orgDefault}`) })}
            </option>
            {COLLAB_ROLES.map((r) => (
              <option key={r} value={r}>
                {t(`collabs.${r}`)}
              </option>
            ))}
          </select>
          <Button
            size="sm"
            disabled={busy || role === (repo.member_role ?? "")}
            onClick={() => void save()}
          >
            {t("common.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
