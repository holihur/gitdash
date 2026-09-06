import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { CircleDot, Copy, FolderGit2, Search, Star, User } from "lucide-react";
import { api, cloneCommand, type GlobalSearchResult, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import Pagination from "@/components/ui/pagination";
import { copyText, formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

export default function Explore() {
  const { t, lang, to } = useI18n();
  const [repos, setRepos] = useState<Repo[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<GlobalSearchResult | null>(null);
  const [searching, setSearching] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.listExplore(pageSize, (page - 1) * pageSize);
      setRepos(r.items);
      setTotal(r.total);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => {
    if (query.trim()) return;
    setResults(null);
    load();
  }, [load, query]);

  useEffect(() => {
    const q = query.trim();
    if (!q) return;
    const timer = setTimeout(async () => {
      setSearching(true);
      try {
        setResults(await api.globalSearch(q));
        setError("");
      } catch (e) {
        toast.error(apiErrorMsg(to, e));
      } finally {
        setSearching(false);
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [query, to]);

  const onPageChange = (p: number) => setPage(p);
  const onPageSizeChange = (s: number) => {
    setPageSize(s);
    setPage(1);
  };

  const toggleStar = async (repo: Repo) => {
    try {
      const r = await api.star(repo.owner, repo.name);
      setRepos((prev) =>
        prev.map((x) =>
          x.id === repo.id ? { ...x, starred: r.starred, stars: r.stars } : x,
        ),
      );
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const copy = (text: string) => {
    copyText(text)
      .then(() => toast.success(t("common.copied")))
      .catch(() => toast.error(t("common.copyFailed")));
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("explore.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("explore.subtitle")}</p>
      </div>

      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="pl-9"
          placeholder={t("explore.searchPlaceholder")}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {searching && <Skeleton className="h-10 w-full" />}

      {results && (
        <div className="space-y-4">
          <h2 className="text-sm font-medium text-muted-foreground">
            {t("explore.searchResults", {
              repos: results.repos.length,
              issues: results.issues.length,
              users: results.users.length,
            })}
          </h2>
          {results.repos.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs font-medium uppercase text-muted-foreground">
                {t("explore.sectionRepos")}
              </p>
              {results.repos.map((r) => (
                <Link
                  key={`${r.owner}/${r.name}`}
                  to={`/repo/${r.owner}/${r.name}`}
                  className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted"
                >
                  <FolderGit2 className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="font-medium">{r.owner}/{r.name}</span>
                  <span className="min-w-0 flex-1 truncate text-muted-foreground">
                    {r.description}
                  </span>
                </Link>
              ))}
            </div>
          )}
          {results.issues.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs font-medium uppercase text-muted-foreground">
                {t("explore.sectionIssues")}
              </p>
              {results.issues.map((i) => (
                <Link
                  key={`${i.owner}/${i.repo}#${i.number}`}
                  to={`/repo/${i.owner}/${i.repo}?tab=issues`}
                  className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted"
                >
                  <CircleDot
                    className={`h-4 w-4 shrink-0 ${i.state === "open" ? "text-green-600" : "text-purple-600"}`}
                  />
                  <span className="font-medium">{i.title}</span>
                  <span className="text-muted-foreground">
                    {i.owner}/{i.repo}#{i.number}
                  </span>
                </Link>
              ))}
            </div>
          )}
          {results.users.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs font-medium uppercase text-muted-foreground">
                {t("explore.sectionUsers")}
              </p>
              {results.users.map((u) => (
                <div key={`${u.kind}:${u.name}`} className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm">
                  <User className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="font-medium">{u.name}</span>
                  {u.kind === "org" && <Badge variant="secondary" className="font-normal">{t("explore.org")}</Badge>}
                  {u.display && <span className="text-muted-foreground">{u.display}</span>}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {t("repos.loadFailed", { error })}
          </CardContent>
        </Card>
      )}

      {!results && loading && !error && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Card key={i}>
              <CardHeader className="pb-3">
                <Skeleton className="h-6 w-40" />
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-2/3" />
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex gap-2">
                  <Skeleton className="h-5 w-28" />
                  <Skeleton className="h-5 w-16" />
                </div>
                <Skeleton className="h-8 w-full" />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {!results && !loading && !error && repos.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <FolderGit2 className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("explore.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("explore.emptyHint")}</p>
          </CardContent>
        </Card>
      )}

      {!results && !loading && !error && repos.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {repos.map((repo) => (
            <Card key={`${repo.owner}/${repo.name}`} className="flex min-w-0 flex-col">
              <CardHeader className="pb-3">
                <CardTitle className="min-w-0 text-lg">
                  <Link to={`/repo/${repo.owner}/${repo.name}`} className="block truncate hover:underline">
                    {repo.owner}/{repo.name}
                  </Link>
                </CardTitle>
                <CardDescription className="min-h-5">
                  <span className="line-clamp-2 block">
                    {repo.description || t("common.noDescription")}
                  </span>
                </CardDescription>
              </CardHeader>
              <CardContent className="mt-auto space-y-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="secondary" className="font-normal">
                    {formatDate(repo.created_at, lang === "zh-CN" ? "zh-CN" : "en-US")}
                  </Badge>
                  <Button
                    variant={repo.starred ? "default" : "outline"}
                    size="sm"
                    className="h-6 gap-1 px-2 text-xs"
                    onClick={() => toggleStar(repo)}
                  >
                    <Star className="h-3 w-3" />
                    {repo.stars ?? 0}
                  </Button>
                </div>
                <div className="flex items-center gap-2 rounded-md border bg-muted/40 px-2 py-1.5">
                  <code className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                    {cloneCommand(repo.owner, repo.name)}
                  </code>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6 shrink-0"
                    onClick={() => copy(cloneCommand(repo.owner, repo.name))}
                  >
                    <Copy className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {!results && !loading && !error && total > 0 && (
        <Pagination
          page={page}
          pageSize={pageSize}
          total={total}
          onPageChange={onPageChange}
          onPageSizeChange={onPageSizeChange}
        />
      )}
    </div>
  );
}
