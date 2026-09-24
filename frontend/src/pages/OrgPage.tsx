import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  Building2,
  CalendarDays,
  FolderGit2,
  Package,
  Settings2,
  Star,
  Trash2,
  UserMinus,
  UserPlus,
} from "lucide-react";
import { api, type OrgProfile, type Repo, type UserSummary } from "@/lib/api";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import ConfirmDialog from "@/components/confirm-dialog";
import { ProfileHero } from "@/components/profile-hero";
import { BadgeStrip } from "@/components/badge-strip";
import { cn, formatDate } from "@/lib/utils";
import { RelativeTime } from "@/components/relative-time";

type View = "repos" | "members" | "followers";

export default function OrgPage() {
  const { org = "" } = useParams();
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
  const nav = useNavigate();
  const [profile, setProfile] = useState<OrgProfile | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [view, setView] = useState<View>("repos");
  const [followers, setFollowers] = useState<UserSummary[] | null>(null);
  const [newMember, setNewMember] = useState("");
  const [newRole, setNewRole] = useState("member");
  const [adding, setAdding] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const isOwner = profile?.role === "owner";
  const isMember = !!profile?.role;

  const load = useCallback(async () => {
    setError("");
    try {
      setProfile(await api.getOrgProfile(org));
    } catch (e) {
      setError(apiErrorMsg(to, e));
    }
  }, [org, to]);
  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (view !== "followers") {
      setFollowers(null);
      return;
    }
    let alive = true;
    setFollowers(null);
    api
      .listOrgFollowers(org)
      .then((u) => alive && setFollowers(u))
      .catch((e) => alive && toast.error(apiErrorMsg(to, e)));
    return () => {
      alive = false;
    };
  }, [view, org, to]);

  const toggleFollow = async () => {
    if (!profile) return;
    setBusy(true);
    try {
      const r = profile.is_following
        ? await api.unfollowOrg(profile.name)
        : await api.followOrg(profile.name);
      setProfile({ ...profile, followers: r.followers, is_following: r.is_following });
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const addMember = async () => {
    if (!profile) return;
    setAdding(true);
    try {
      await api.addOrgMember(profile.name, newMember.trim(), newRole);
      toast.success(t("orgs.memberAdded", { name: newMember.trim() }));
      setNewMember("");
      setNewRole("member");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setAdding(false);
    }
  };

  const removeMember = async (username: string) => {
    if (!profile) return;
    try {
      await api.removeOrgMember(profile.name, username);
      toast.success(t("orgs.memberRemoved", { name: username }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const deleteOrg = async () => {
    if (!profile) return;
    setConfirmDelete(false);
    try {
      await api.deleteOrg(profile.name);
      toast.success(t("orgs.deleted", { name: profile.name }));
      nav("/orgs");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  if (error) {
    return (
      <Card className="border-destructive">
        <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
      </Card>
    );
  }
  if (!profile) return <p className="py-10 text-center text-sm text-muted-foreground">…</p>;

  const tabs: { key: View; label: string; count: number }[] = [
    { key: "repos", label: t("orgs.repos"), count: profile.repos.length },
    { key: "members", label: t("orgs.members"), count: profile.members.length },
    { key: "followers", label: t("orgs.followers"), count: profile.followers },
  ];

  return (
    <div className="space-y-6">
      <ProfileHero
        coverUrl={profile.cover_url}
        avatar={
          <div className="flex h-20 w-20 items-center justify-center rounded-2xl bg-muted ring-4 ring-background">
            <Building2 className="h-9 w-9 text-muted-foreground" />
          </div>
        }
        title={profile.display || profile.name}
        subtitle={profile.name}
        badges={<BadgeStrip kind="org" owner={profile.name} />}
        badge={isMember ? <Badge variant="secondary">{profile.role}</Badge> : undefined}
        bio={profile.bio}
        meta={
          <>
            <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <CalendarDays className="h-3.5 w-3.5" />
              {t("orgs.since", { date: formatDate(profile.created_at, locale) })}
            </p>
            <p className="flex flex-wrap items-center gap-4 pt-1 text-sm text-muted-foreground">
              <span>
                <span className="font-medium text-foreground">{profile.followers}</span>{" "}
                {t("orgs.followers").toLowerCase()}
              </span>
              <span>
                <span className="font-medium text-foreground">{profile.members.length}</span>{" "}
                {t("orgs.members").toLowerCase()}
              </span>
            </p>
          </>
        }
        actions={
          <>
            {isOwner && (
              <Button asChild variant="outline" size="sm" className="gap-1.5">
                <Link to={`/orgs/${encodeURIComponent(profile.name)}/settings`}>
                  <Settings2 className="h-4 w-4" />
                  {t("orgs.editOrg")}
                </Link>
              </Button>
            )}
            {isOwner && (
              <Button
                variant="outline"
                size="sm"
                className="gap-1.5 text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="h-4 w-4" />
                {t("orgs.deleteOrg")}
              </Button>
            )}
            <Button asChild variant="outline" size="sm" className="gap-1.5">
              <Link to={`/packages?owner=${encodeURIComponent(profile.name)}`}>
                <Package className="h-4 w-4" />
                {t("packages.title")}
              </Link>
            </Button>
            <Button
              variant={profile.is_following ? "outline" : "default"}
              size="sm"
              className="gap-1.5"
              disabled={busy}
              onClick={toggleFollow}
            >
              {profile.is_following ? (
                <>
                  <UserMinus className="h-4 w-4" />
                  {t("user.unfollow")}
                </>
              ) : (
                <>
                  <UserPlus className="h-4 w-4" />
                  {t("user.follow")}
                </>
              )}
            </Button>
          </>
        }
      />

      <div className="flex flex-wrap gap-1 border-b">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => setView(tab.key)}
            className={cn(
              "-mb-px border-b-2 px-3 py-2 text-sm",
              view === tab.key
                ? "border-primary font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {tab.label}
            <Badge variant="secondary" className="ml-2 font-normal">
              {tab.count}
            </Badge>
          </button>
        ))}
      </div>

      {view === "repos" ? (
        profile.repos.length === 0 ? (
          <p className="rounded-lg border border-dashed py-12 text-center text-sm text-muted-foreground">
            {t("orgs.noRepos")}
          </p>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {profile.repos.map((repo) => (
              <RepoCard key={`${repo.owner}/${repo.name}`} repo={repo} />
            ))}
          </div>
        )
      ) : view === "members" ? (
        <div className="space-y-3">
          <div className="divide-y divide-border rounded-lg border">
            {profile.members.map((m) => (
              <div key={m.username} className="flex items-center gap-3 px-3 py-2">
                <Link
                  to={`/users/${m.username}`}
                  className="flex min-w-0 flex-1 items-center gap-3 hover:underline"
                >
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
                    {m.username.slice(0, 1).toUpperCase()}
                  </span>
                  <span className="min-w-0 truncate text-sm font-medium">{m.username}</span>
                </Link>
                <Badge variant="secondary">{m.role}</Badge>
                {isOwner && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-destructive hover:text-destructive"
                    title={t("common.delete")}
                    onClick={() => removeMember(m.username)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                )}
              </div>
            ))}
          </div>
          {isOwner && (
            <div className="flex flex-col gap-2 sm:flex-row">
              <Input
                placeholder={t("orgs.memberUser")}
                value={newMember}
                onChange={(e) => setNewMember(e.target.value)}
              />
              <select
                value={newRole}
                onChange={(e) => setNewRole(e.target.value)}
                className="h-10 rounded-md border border-input bg-background px-2 text-sm"
              >
                <option value="member">member</option>
                <option value="owner">owner</option>
              </select>
              <Button onClick={addMember} disabled={adding || !newMember.trim()}>
                {t("common.add")}
              </Button>
            </div>
          )}
        </div>
      ) : followers === null ? (
        <p className="py-10 text-center text-sm text-muted-foreground">…</p>
      ) : followers.length === 0 ? (
        <p className="rounded-lg border border-dashed py-12 text-center text-sm text-muted-foreground">
          {t("orgs.noFollowers")}
        </p>
      ) : (
        <div className="divide-y divide-border rounded-lg border">
          {followers.map((u) => (
            <Link
              key={u.username}
              to={`/users/${u.username}`}
              className="flex items-center gap-3 px-3 py-2 hover:bg-muted"
            >
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
                {u.username.slice(0, 1).toUpperCase()}
              </span>
              <span className="min-w-0 truncate text-sm font-medium">{u.username}</span>
            </Link>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        description={t("orgs.confirmDelete", { name: profile.name })}
        onConfirm={deleteOrg}
      />
    </div>
  );
}

function RepoCard({ repo }: { repo: Repo }) {
  const { t, lang } = useI18n();
  const locale = dateLocale(lang);
  return (
    <Card className="flex min-w-0 flex-col">
      <CardContent className="space-y-2 pt-4">
        <div className="flex items-center gap-2">
          <FolderGit2 className="h-4 w-4 shrink-0 text-muted-foreground" />
          <Link
            to={`/repo/${repo.owner}/${repo.name}`}
            className="min-w-0 truncate font-medium hover:underline"
          >
            {repo.name}
          </Link>
          {repo.private && (
            <Badge variant="secondary" className="font-normal">
              {t("repo.privateRepo")}
            </Badge>
          )}
        </div>
        <p className="line-clamp-2 min-h-5 text-sm text-muted-foreground">
          {repo.description || t("common.noDescription")}
        </p>
        <BadgeStrip kind="repo" owner={repo.owner} repo={repo.name} />
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>
            <RelativeTime iso={repo.created_at} locale={locale} />
          </span>
          {typeof repo.stars === "number" && (
            <span className="flex items-center gap-1">
              <Star className="h-3 w-3" />
              {repo.stars}
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
