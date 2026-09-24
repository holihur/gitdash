import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { ShieldCheck } from "lucide-react";
import { api, type AccessEntry, type OrgTeam, type RepoTeamGrant } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { COLLAB_ROLES } from "@/lib/repo-role";

/** 仓库的团队授权 + 权限审计（仅 admin 可见/可改）。 */
export function RepoTeamAccess({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [teams, setTeams] = useState<OrgTeam[]>([]);
  const [grants, setGrants] = useState<RepoTeamGrant[]>([]);
  const [entries, setEntries] = useState<AccessEntry[]>([]);
  const [pickTeam, setPickTeam] = useState<number>(0);
  const [pickRole, setPickRole] = useState<string>("read");

  const load = useCallback(async () => {
    try {
      const [ts, gs, es] = await Promise.all([
        api.listOrgTeams(owner).catch(() => [] as OrgTeam[]),
        api.repoTeamGrants(owner, name).catch(() => [] as RepoTeamGrant[]),
        api.repoAccess(owner, name).catch(() => [] as AccessEntry[]),
      ]);
      setTeams(ts);
      setGrants(gs);
      setEntries(es);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);
  useEffect(() => {
    void load();
  }, [load]);

  const granted = new Set(grants.map((g) => g.team_id));
  const available = teams.filter((tm) => !granted.has(tm.id));

  const grant = async () => {
    if (!pickTeam) return;
    try {
      await api.grantRepoTeam(owner, name, pickTeam, pickRole);
      setPickTeam(0);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const revoke = async (teamId: number) => {
    try {
      await api.revokeRepoTeam(owner, name, teamId);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" />
          {t("repoAccess.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {teams.length > 0 && (
          <div className="space-y-2">
            <p className="text-xs font-medium text-muted-foreground">{t("teams.repoGrants")}</p>
            {grants.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {grants.map((g) => (
                  <span
                    key={g.team_id}
                    className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
                  >
                    {g.team_name}
                    <Badge variant="secondary" className="h-4 px-1 text-[10px]">
                      {t(`collabs.${g.permission}`)}
                    </Badge>
                    <button
                      type="button"
                      className="text-destructive"
                      title={t("teams.revoke")}
                      onClick={() => void revoke(g.team_id)}
                    >
                      ×
                    </button>
                  </span>
                ))}
              </div>
            )}
            {available.length > 0 && (
              <div className="flex flex-wrap items-center gap-2">
                <select
                  aria-label={t("teams.pickTeam")}
                  className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                  value={pickTeam}
                  onChange={(e) => setPickTeam(Number(e.target.value))}
                >
                  <option value={0}>{t("teams.pickTeam")}</option>
                  {available.map((tm) => (
                    <option key={tm.id} value={tm.id}>
                      {tm.name}
                    </option>
                  ))}
                </select>
                <select
                  aria-label={t("collabs.permission")}
                  className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                  value={pickRole}
                  onChange={(e) => setPickRole(e.target.value)}
                >
                  {COLLAB_ROLES.map((r) => (
                    <option key={r} value={r}>
                      {t(`collabs.${r}`)}
                    </option>
                  ))}
                </select>
                <Button size="sm" onClick={() => void grant()} disabled={!pickTeam}>
                  {t("teams.grant")}
                </Button>
              </div>
            )}
          </div>
        )}

        <div className="space-y-1">
          <p className="text-xs font-medium text-muted-foreground">{t("teams.audit")}</p>
          {entries.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("teams.auditEmpty")}</p>
          ) : (
            <div className="divide-y divide-border rounded-md border text-sm">
              {entries.map((e, i) => (
                <div key={`${e.subject}-${e.source}-${i}`} className="flex items-center gap-2 px-2 py-1.5">
                  <span className="min-w-0 flex-1 truncate font-medium">{e.subject}</span>
                  <Badge variant="outline" className="shrink-0 font-normal">
                    {t(`collabs.${e.role}`)}
                  </Badge>
                  <span className="shrink-0 text-xs text-muted-foreground">{e.source}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
