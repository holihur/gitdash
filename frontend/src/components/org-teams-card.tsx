import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Trash2, UsersRound } from "lucide-react";
import { api, type OrgTeam } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

/** 组织设置里的团队管理：建/删团队、增删成员。 */
export function OrgTeamsCard({ org }: { org: string }) {
  const { t, to } = useI18n();
  const [teams, setTeams] = useState<OrgTeam[]>([]);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [members, setMembers] = useState<Record<number, string[] | undefined>>({});
  const [draft, setDraft] = useState<Record<number, string>>({});

  const load = useCallback(async () => {
    try {
      setTeams(await api.listOrgTeams(org));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [org, to]);
  useEffect(() => {
    void load();
  }, [load]);

  const create = async () => {
    const n = name.trim();
    if (!n) return;
    setBusy(true);
    try {
      await api.createOrgTeam(org, n);
      setName("");
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const removeTeam = async (id: number) => {
    try {
      await api.deleteOrgTeam(org, id);
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const toggleMembers = async (id: number) => {
    if (members[id]) {
      setMembers((m) => ({ ...m, [id]: undefined }));
      return;
    }
    try {
      const r = await api.listOrgTeamMembers(org, id);
      setMembers((m) => ({ ...m, [id]: r.members }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const addMember = async (id: number) => {
    const u = (draft[id] ?? "").trim();
    if (!u) return;
    try {
      await api.addOrgTeamMember(org, id, u);
      setDraft((s) => ({ ...s, [id]: "" }));
      const r = await api.listOrgTeamMembers(org, id);
      setMembers((m) => ({ ...m, [id]: r.members }));
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const removeMember = async (id: number, username: string) => {
    try {
      await api.removeOrgTeamMember(org, id, username);
      const r = await api.listOrgTeamMembers(org, id);
      setMembers((m) => ({ ...m, [id]: r.members }));
      await load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("teams.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">{t("teams.hint")}</p>
        <div className="flex gap-2">
          <Input
            value={name}
            placeholder={t("teams.namePlaceholder")}
            maxLength={100}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void create()}
          />
          <Button onClick={() => void create()} disabled={busy || !name.trim()}>
            {t("teams.create")}
          </Button>
        </div>

        {teams.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("teams.empty")}</p>
        ) : (
          <div className="grid gap-2">
            {teams.map((team) => (
              <div key={team.id} className="rounded-md border p-2">
                <div className="flex items-center gap-2">
                  <UsersRound className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">{team.name}</span>
                  <Button size="sm" variant="outline" onClick={() => void toggleMembers(team.id)}>
                    {t("teams.members", { count: team.member_count })}
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8 text-destructive hover:text-destructive"
                    title={t("teams.delete")}
                    onClick={() => void removeTeam(team.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
                {members[team.id] !== undefined && (
                  <div className="mt-2 space-y-2 border-t pt-2">
                    <div className="flex flex-wrap gap-1.5">
                      {(members[team.id] ?? []).map((u) => (
                        <span
                          key={u}
                          className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
                        >
                          {u}
                          <button
                            type="button"
                            className="text-destructive"
                            onClick={() => void removeMember(team.id, u)}
                          >
                            ×
                          </button>
                        </span>
                      ))}
                      {(members[team.id] ?? []).length === 0 && (
                        <span className="text-xs text-muted-foreground">{t("teams.noMembers")}</span>
                      )}
                    </div>
                    <div className="flex gap-2">
                      <Input
                        className="h-8"
                        placeholder={t("teams.memberPlaceholder")}
                        value={draft[team.id] ?? ""}
                        onChange={(e) => setDraft((s) => ({ ...s, [team.id]: e.target.value }))}
                        onKeyDown={(e) => e.key === "Enter" && void addMember(team.id)}
                      />
                      <Button size="sm" onClick={() => void addMember(team.id)}>
                        {t("teams.addMember")}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
