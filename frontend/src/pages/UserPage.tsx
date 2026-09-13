import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";
import { CalendarDays, FolderGit2, Settings, Star, UserMinus, UserPlus } from "lucide-react";
import { api, type Repo, type UserProfile, type UserSummary } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Avatar } from "@/components/avatar";
import { cn, formatDate } from "@/lib/utils";

type View = "repos" | "followers" | "following";

export default function UserPage() {
  const { username = "" } = useParams();
  const { t, to, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [view, setView] = useState<View>("repos");
  const [people, setPeople] = useState<UserSummary[] | null>(null);

  const load = useCallback(async () => {
    setError("");
    setProfile(null);
    try {
      setProfile(await api.getUser(username));
    } catch (e) {
      setError(apiErrorMsg(to, e));
    }
  }, [username, to]);

  useEffect(() => {
    load();
  }, [load]);

  // 切换 followers/following 时按需加载列表
  useEffect(() => {
    if (view === "repos") {
      setPeople(null);
      return;
    }
    let alive = true;
    setPeople(null);
    const req = view === "followers" ? api.listFollowers(username) : api.listFollowing(username);
    req
      .then((u) => alive && setPeople(u))
      .catch((e) => alive && toast.error(apiErrorMsg(to, e)));
    return () => {
      alive = false;
    };
  }, [view, username, to]);

  const toggleFollow = async () => {
    if (!profile) return;
    setBusy(true);
    try {
      const r = profile.is_following
        ? await api.unfollowUser(profile.username)
        : await api.followUser(profile.username);
      setProfile({ ...profile, ...r });
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
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
    { key: "repos", label: t("user.repositories"), count: profile.repos.length },
    { key: "followers", label: t("user.followers"), count: profile.followers },
    { key: "following", label: t("user.following"), count: profile.following },
  ];

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <Avatar username={profile.username} size={80} />
        <div className="min-w-0 flex-1 space-y-1">
          <h1 className="truncate text-2xl font-bold">{profile.username}</h1>
          <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <CalendarDays className="h-3.5 w-3.5" />
            {t("user.memberSince", { date: formatDate(profile.created_at, locale) })}
          </p>
          <p className="flex flex-wrap items-center gap-4 pt-1 text-sm text-muted-foreground">
            <span>
              <span className="font-medium text-foreground">{profile.followers}</span>{" "}
              {t("user.followers")}
            </span>
            <span>
              <span className="font-medium text-foreground">{profile.following}</span>{" "}
              {t("user.following")}
            </span>
          </p>
        </div>
        {profile.is_self ? (
          <Button asChild variant="outline" size="sm" className="gap-1.5 self-start">
            <Link to="/profile">
              <Settings className="h-4 w-4" />
              {t("user.editProfile")}
            </Link>
          </Button>
        ) : (
          <Button
            variant={profile.is_following ? "outline" : "default"}
            size="sm"
            className="gap-1.5 self-start"
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
        )}
      </div>

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
            {t("user.noRepos")}
          </p>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {profile.repos.map((repo) => (
              <RepoCard key={`${repo.owner}/${repo.name}`} repo={repo} />
            ))}
          </div>
        )
      ) : people === null ? (
        <p className="py-10 text-center text-sm text-muted-foreground">…</p>
      ) : people.length === 0 ? (
        <p className="rounded-lg border border-dashed py-12 text-center text-sm text-muted-foreground">
          {t(view === "followers" ? "user.noFollowers" : "user.noFollowing")}
        </p>
      ) : (
        <div className="divide-y divide-border rounded-lg border">
          {people.map((u) => (
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
    </div>
  );
}

function RepoCard({ repo }: { repo: Repo }) {
  const { t, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
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
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>{formatDate(repo.created_at, locale)}</span>
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
