import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { toast } from "sonner";
import {
  Copy,
  Eye,
  GitBranch,
  GitFork,
  HardDrive,
  MoreVertical,
  RefreshCw,
  Star,
} from "lucide-react";
import { api, cloneCommand, type Blame, type Blob, type Branch, type Commit, type Repo, type Tag, type TreeEntry } from "@/lib/api";
import { buildRepoPath, parseRepoRoute, type RepoCodeKind, type RepoTab } from "@/lib/repo-url";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { TabsListOverflow } from "@/components/ui/tabs-overflow";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { cn, copyText, formatSize } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import FileOpDialog, { type FileOp } from "@/components/file-op-dialog";
import MirrorDialog from "@/components/mirror-dialog";
import RefsDialog from "@/components/refs-dialog";

const CodeTab = lazy(() => import("./repoview/code-tab"));
const CommitsTab = lazy(() => import("./repoview/commits-tab"));
const IssuesTab = lazy(() => import("./repoview/issues-tab"));
const PullsTab = lazy(() => import("./repoview/pulls-tab"));
const PipelineTab = lazy(() => import("./repoview/pipeline-tab"));
const CopilotTab = lazy(() => import("./repoview/copilot-tab"));
const ReleasesTab = lazy(() => import("./repoview/releases-tab"));
const ProjectsTab = lazy(() => import("./repoview/projects-tab"));
const SettingsTab = lazy(() => import("./repoview/settings-tab"));

export default function RepoView() {
  const { t, lang, to } = useI18n();
  const locale = dateLocale(lang);
  const { owner = "", name = "", "*": splat = "" } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

  // 路径化路由派生：/repo/:owner/:name/{tree|blob|blame}/... 及其余 tab。
  // ref / line 仍走查询参数（ref 可能包含 `/`，放路径里会与文件路径歧义）。
  const route = useMemo(() => parseRepoRoute(splat), [splat]);
  const tab: RepoTab = route.tab;
  const fileParam = route.tab === "code" && route.kind !== "tree" ? route.path : "";
  const path =
    route.tab === "code" && route.kind === "tree" && route.path ? route.path.split("/") : [];
  const currentDir = route.tab === "code" && route.kind === "tree" ? route.path : "";
  const blameParam = route.tab === "code" && route.kind === "blame";
  const urlRef = searchParams.get("ref") ?? "";
  const lineParam = Number(searchParams.get("line")) || null;

  // 兼容既有 `setParams` 语义（CodeTab / tab 切换均调用），把变更翻译为路径化导航。
  const setParams = useCallback(
    (patch: Record<string, string | null>) => {
      const nextTab: RepoTab = "tab" in patch ? ((patch.tab as RepoTab) ?? "code") : tab;
      let kind: RepoCodeKind = route.kind;
      let dirPath = route.kind === "tree" ? route.path : "";
      let filePath = route.kind !== "tree" ? route.path : "";
      let nextBlame = route.kind === "blame";
      let nextRef = urlRef;
      let nextLine: number | null = lineParam;
      let nextHash = "";

      if ("path" in patch) dirPath = patch.path ?? "";
      if ("file" in patch) {
        filePath = patch.file ?? "";
        kind = filePath ? (nextBlame ? "blame" : "blob") : "tree";
      }
      if ("blame" in patch) {
        nextBlame = patch.blame === "1" || patch.blame === "true";
        if (filePath) kind = nextBlame ? "blame" : "blob";
      }
      if ("ref" in patch) nextRef = patch.ref ?? "";
      if ("line" in patch) nextLine = patch.line ? Number(patch.line) : null;
      // 支持跨文件锚点：patch.hash 不进查询参数，只拼成 URL 片段。
      if ("hash" in patch) nextHash = patch.hash ?? "";

      const pathname =
        nextTab === "code"
          ? buildRepoPath(owner, name, {
              tab: "code",
              kind,
              path: kind === "tree" ? dirPath : filePath,
            })
          : buildRepoPath(owner, name, { tab: nextTab });

      const next = new URLSearchParams(searchParams);
      if (nextRef) next.set("ref", nextRef);
      else next.delete("ref");
      if (nextTab === "code" && nextLine && filePath) next.set("line", String(nextLine));
      else next.delete("line");
      const qs = next.toString();
      navigate(`${pathname}${qs ? `?${qs}` : ""}${nextHash ? `#${nextHash}` : ""}`);
    },
    [owner, name, tab, route, urlRef, lineParam, searchParams, navigate],
  );

  // 兼容旧的查询参数式 URL（?tab=&path=&file=&blame=），自动重定向到路径化地址。
  useEffect(() => {
    const legacyTab = searchParams.get("tab");
    const legacyPath = searchParams.get("path") ?? "";
    const legacyFile = searchParams.get("file") ?? "";
    const legacyBlame = searchParams.get("blame") === "1";
    if (!legacyTab && !legacyPath && !legacyFile && !legacyBlame) return;
    const kind: RepoCodeKind = legacyFile ? (legacyBlame ? "blame" : "blob") : "tree";
    const pathname =
      legacyTab && legacyTab !== "code"
        ? buildRepoPath(owner, name, { tab: legacyTab as RepoTab })
        : buildRepoPath(owner, name, { tab: "code", kind, path: legacyFile || legacyPath });
    const next = new URLSearchParams(searchParams);
    next.delete("tab");
    next.delete("path");
    next.delete("file");
    next.delete("blame");
    const qs = next.toString();
    navigate(`${pathname}${qs ? `?${qs}` : ""}`, { replace: true });
  }, [searchParams, navigate, owner, name]);

  const [repo, setRepo] = useState<Repo | null>(null);
  // 仓库角色由后端按 owner / 协作者 / 组织角色计算；组织仓库的 owner 也是"owner"，
  // 不能用 me === owner 判断（组织仓库 owner 是组织名）。
  const isOwner = repo?.role === "owner";
  const [branches, setBranches] = useState<Branch[]>([]);
  const [entries, setEntries] = useState<TreeEntry[]>([]);
  const [treeLatestCommit, setTreeLatestCommit] = useState<Commit | null>(null);
  const [blob, setBlob] = useState<Blob | null>(null);
  const [blame, setBlame] = useState<Blame | null>(null);
  const [error, setError] = useState("");
  const [missing, setMissing] = useState(false);
  const [readmeContent, setReadmeContent] = useState<string | null>(null);
  const [fileOp, setFileOp] = useState<FileOp | null>(null);
  const [loadTick, setLoadTick] = useState(0);
  const [tags, setTags] = useState<Tag[]>([]);
  const [refsOpen, setRefsOpen] = useState(false);
  const [starBusy, setStarBusy] = useState(false);
  const [watchBusy, setWatchBusy] = useState(false);
  const [forkOpen, setForkOpen] = useState(false);
  const [forkName, setForkName] = useState("");
  const [forkBusy, setForkBusy] = useState(false);
  const [mirrorOpen, setMirrorOpen] = useState(false);
  const [pendingRemove, setPendingRemove] = useState<{ path: string; isDir: boolean } | null>(null);

  const ref =
    urlRef ||
    branches.find((b) => b.is_head)?.name ||
    repo?.default_branch ||
    branches[0]?.name ||
    "";

  const refreshRefs = useCallback(async () => {
    try {
      const [bs, ts] = await Promise.all([api.branches(owner, name), api.listTags(owner, name)]);
      setBranches(bs);
      setTags(ts);
    } catch {
      /* ignore */
    }
  }, [owner, name]);

  const copy = useCallback(
    (text: string) => {
      copyText(text)
        .then(() => toast.success(t("common.copied")))
        .catch(() => toast.error(t("common.copyFailed")));
    },
    [t],
  );

  const toggleStar = async () => {
    if (!repo) return;
    setStarBusy(true);
    try {
      const s = repo.starred ? await api.unstar(owner, name) : await api.star(owner, name);
      setRepo({ ...repo, starred: s.starred, stars: s.stars });
      toast.success(t(repo.starred ? "social.unstarred" : "social.starred"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setStarBusy(false);
    }
  };

  const toggleWatch = async () => {
    if (!repo) return;
    setWatchBusy(true);
    try {
      const s = repo.watching ? await api.unwatch(owner, name) : await api.watch(owner, name);
      setRepo({ ...repo, watching: s.watching, watchers: s.watchers });
      toast.success(
        t(repo.watching ? "social.unwatched" : "social.watched", { name: `${owner}/${name}` }),
      );
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setWatchBusy(false);
    }
  };

  const openFork = () => {
    setForkName(name);
    setForkOpen(true);
  };

  const doFork = async () => {
    setForkBusy(true);
    try {
      const r = await api.forkRepo(owner, name, { name: forkName.trim() || name });
      toast.success(t("social.forked", { name: r.name }));
      setForkOpen(false);
      navigate(`/repo/${r.owner}/${r.name}`);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setForkBusy(false);
    }
  };

  useEffect(() => {
    (async () => {
      try {
        const [r, bs] = await Promise.all([api.getRepo(owner, name), api.branches(owner, name)]);
        setRepo(r);
        setBranches(bs);
        api.listTags(owner, name).then(setTags).catch(() => undefined);
      } catch (e) {
        setMissing(true);
        setError(e instanceof Error ? e.message : String(e));
      }
    })();
  }, [name, owner]);

  const loadTree = useCallback(async () => {
    void loadTick;
    if (!ref) return;
    try {
      const data = await api.tree(owner, name, ref, currentDir);
      setEntries(data.entries);
      setTreeLatestCommit(data.latest_commit ?? null);
      setBlob(null);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [name, owner, ref, currentDir, loadTick]);

  useEffect(() => {
    loadTree();
  }, [loadTree]);

  // blob 内容跟随 ?file= 参数加载
  useEffect(() => {
    let alive = true;
    if (!fileParam || !ref) {
      setBlob(null);
      return;
    }
    api
      .blob(owner, name, ref, fileParam)
      .then((b) => {
        if (alive) setBlob(b);
      })
      .catch((err) => {
        if (!alive) return;
        setBlob(null);
        // 目录链接可能没有尾随 `/`（Markdown 相对链接常见）：先按目录尝试。
        void (async () => {
          try {
            await api.tree(owner, name, ref, fileParam);
            if (!alive) return;
            setParams({ path: fileParam, file: null, line: null, blame: null });
          } catch {
            if (!alive) return;
            setParams({ file: null });
            toast.error(apiErrorMsg(to, err));
          }
        })();
      });
    return () => {
      alive = false;
    };
  }, [fileParam, ref, owner, name, setParams, to]);

  // blame 数据跟随 ?blame=1 & ?file= 参数加载
  useEffect(() => {
    let alive = true;
    if (!blameParam || !fileParam || !ref) {
      setBlame(null);
      return;
    }
    api
      .blame(owner, name, ref, fileParam)
      .then((b) => {
        if (alive) setBlame(b);
      })
      .catch((e) => {
        if (!alive) return;
        setBlame(null);
        toast.error(apiErrorMsg(to, e));
      });
    return () => {
      alive = false;
    };
  }, [blameParam, fileParam, ref, owner, name, to]);

  // 目录 README：列表底部渲染
  const readmeEntry = entries.find(
    (e) => e.type === "blob" && /^readme(\..+)?$/i.test(e.name),
  );
  useEffect(() => {
    let alive = true;
    if (!ref || blob || !readmeEntry) {
      setReadmeContent(null);
      return;
    }
    const full = currentDir ? currentDir + "/" + readmeEntry.name : readmeEntry.name;
    api
      .blob(owner, name, ref, full)
      .then((b) => alive && b.encoding === "utf-8" && setReadmeContent(b.content))
      .catch(() => undefined);
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ref, entries, blob, owner, name, currentDir]);

  const openCreateDialog = (kind: "create-file" | "create-dir") => {
    const prefix = currentDir ? currentDir + "/" : "";
    setFileOp({
      kind,
      path: kind === "create-dir" ? prefix : prefix + "",
      content: "",
      branch: ref || branches[0]?.name || "main",
    });
  };

  const openEditDialog = (filePath: string, content: string) => {
    setFileOp({ kind: "edit", path: filePath, content, branch: ref });
  };

  const removeEntry = (targetPath: string, isDir: boolean) => {
    setPendingRemove({ path: targetPath, isDir });
  };

  const confirmRemove = async () => {
    if (!pendingRemove) return;
    const { path: targetPath, isDir } = pendingRemove;
    const branch = ref || branches[0]?.name || "main";
    try {
      await api.createCommit(owner, name, branch, `Delete ${targetPath}`, [
        { path: targetPath, action: isDir ? "delete_tree" : "delete" },
      ]);
      toast.success(t("fops.deleted", { path: targetPath }));
      setPendingRemove(null);
      afterCommit(branch, isDir ? currentDir : undefined);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const afterCommit = async (branch: string, backToDir?: string) => {
    const nextPath =
      backToDir !== undefined ? (backToDir ? backToDir.split("/") : []) : blob ? [] : path;
    setParams({
      ref: branch,
      path: nextPath.length ? nextPath.join("/") : null,
      file: null,
    });
    setLoadTick((n) => n + 1);
    try {
      setBranches(await api.branches(owner, name));
    } catch {
      /* ignore */
    }
  };

  const emptyRepo = branches.length === 0;
  const commands = useMemo(
    () => [
      cloneCommand(owner, name),
      `cd ${name}`,
      `echo "# ${name}" >> README.md`,
      "git add README.md",
      'git commit -m "initial commit"',
      `git push origin ${branches[0]?.name || "main"}`,
    ],
    [name, owner, branches],
  );

  const overflowTabs = useMemo(
    () => [
      { value: "code", label: t("repo.code") },
      { value: "commits", label: t("repo.commits") },
      ...(repo?.has_issues === false ? [] : [{ value: "issues", label: t("issues.title") }]),
      { value: "pulls", label: t("pulls.title") },
      { value: "pipeline", label: t("pipeline.tab") },
      { value: "copilot", label: t("copilot.tab") },
      { value: "releases", label: t("releases.tab") },
      { value: "projects", label: t("projects.tab") },
      ...(isOwner ? [{ value: "settings", label: t("repo.settings") }] : []),
    ],
    [t, isOwner, repo?.has_issues],
  );

  // 关闭 issue 后，若 URL 仍指向 issues tab，回退到 code。
  const issuesDisabled = repo?.has_issues === false;
  const activeTab: RepoTab = issuesDisabled && tab === "issues" ? "code" : tab;
  useEffect(() => {
    if (issuesDisabled && tab === "issues") setParams({ tab: null });
  }, [issuesDisabled, tab, setParams]);

  if (missing) {
    return (
      <Card className="border-destructive">
        <CardContent className="pt-6 text-sm text-destructive">
          {t("repo.notFound", { error })}
        </CardContent>
      </Card>
    );
  }

  if (!repo) {
    return (
      <div className="space-y-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0 space-y-2">
            <Skeleton className="h-8 w-64 max-w-full" />
            <Skeleton className="h-4 w-96 max-w-full" />
          </div>
          <div className="flex flex-wrap gap-2">
            <Skeleton className="h-8 w-24" />
            <Skeleton className="h-8 w-24" />
            <Skeleton className="h-8 w-20" />
          </div>
        </div>
        <Skeleton className="h-10 w-full sm:w-96" />
        <div className="space-y-3 rounded-lg border p-4">
          <Skeleton className="h-9 w-40" />
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-full" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-bold">
            <Link to={`/users/${owner}`} className="hover:underline">
              {owner}
            </Link>
            <span className="text-muted-foreground">/</span>
            {name}
          </h1>
          <p className="text-sm text-muted-foreground">{repo?.description || t("common.noDescription")}</p>
          {repo?.topics && repo.topics.length > 0 && (
            <div className="mt-1 flex flex-wrap gap-1">
              {repo.topics.map((tp) => (
                <Link key={tp} to={`/explore?tag=${encodeURIComponent(tp)}`}>
                  <Badge variant="secondary" className="font-normal">
                    {tp}
                  </Badge>
                </Link>
              ))}
            </div>
          )}
          {repo?.fork_owner && repo?.fork_repo && (
            <p className="text-xs text-muted-foreground">
              {t("social.forkedFrom")}{" "}
              <button
                className="hover:underline"
                onClick={() => navigate(`/repo/${repo.fork_owner}/${repo.fork_repo}`)}
              >
                {repo.fork_owner}/{repo.fork_repo}
              </button>
            </p>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            disabled={watchBusy}
            onClick={toggleWatch}
            title={t("social.watchTitle")}
          >
            <Eye className={cn("h-4 w-4", repo?.watching && "fill-current text-blue-500")} />
            {repo?.watching ? t("social.watchingBtn") : t("social.watch")}
            <span className="text-muted-foreground">{repo?.watchers ?? 0}</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            disabled={starBusy}
            onClick={toggleStar}
          >
            <Star className={cn("h-4 w-4", repo?.starred && "fill-current text-yellow-500")} />
            {repo?.starred ? t("social.starredBtn") : t("social.star")}
            <span className="text-muted-foreground">{repo?.stars ?? 0}</span>
          </Button>
          {typeof repo?.size === "number" && (
            <span
              className="inline-flex h-9 items-center gap-1.5 rounded-md border border-input bg-background px-3 text-sm text-muted-foreground"
              title={t("common.size")}
            >
              <HardDrive className="h-4 w-4" />
              {formatSize(repo.size)}
            </span>
          )}
          {!isOwner && (
            <Button variant="outline" size="sm" className="gap-1.5" onClick={openFork}>
              <GitFork className="h-4 w-4" />
              {t("social.fork")}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            title={t("mirror.title")}
            onClick={() => setMirrorOpen(true)}
          >
            <RefreshCw className="h-4 w-4" />
            {t("mirror.sync")}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" className="w-full gap-2 font-mono text-xs sm:w-auto">
                <GitBranch className="h-3.5 w-3.5" />
                SSH
                <MoreVertical className="ml-auto h-3.5 w-3.5 sm:ml-0" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-80 max-w-[calc(100vw-2rem)]">
              <DropdownMenuLabel className="break-all font-mono text-xs normal-case">
                {cloneCommand(owner, name)}
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => copy(cloneCommand(owner, name))}>
                <Copy />
                {t("repo.copyCloneCommand")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={(v) => setParams({ tab: v === "code" ? null : v })}>
        <TabsListOverflow
          tabs={overflowTabs}
          value={activeTab}
          onValueChange={(v) => setParams({ tab: v === "code" ? null : v })}
          listClassName="w-full sm:w-auto"
        />

        <TabsContent value="code">
          <Suspense fallback={<TabFallback />}>
            <CodeTab
              owner={owner}
              name={name}
              refName={ref}
              locale={locale}
              path={path}
              currentDir={currentDir}
              branches={branches}
              tags={tags}
              entries={entries}
              dirLatestCommit={treeLatestCommit}
              blob={blob}
              blame={blame}
              error={error}
              emptyRepo={emptyRepo}
              readmeContent={readmeContent}
              readmeEntryName={readmeEntry?.name ?? null}
              blameParam={blameParam}
              lineParam={lineParam}
              setParams={setParams}
              commands={commands}
              openRefs={() => setRefsOpen(true)}
              openCreateDialog={openCreateDialog}
              openEditDialog={openEditDialog}
              removeEntry={removeEntry}
              copy={copy}
            />
          </Suspense>
        </TabsContent>

        <TabsContent value="commits">
          <Suspense fallback={<TabFallback />}>
            <CommitsTab owner={owner} name={name} refName={ref} emptyRepo={emptyRepo} role={repo?.role} />
          </Suspense>
        </TabsContent>

        <TabsContent value="issues">
          <Suspense fallback={<TabFallback />}>
            <IssuesTab owner={owner} name={name} role={repo?.role} />
          </Suspense>
        </TabsContent>

          <TabsContent value="pulls">
            <Suspense fallback={<TabFallback />}>
              <PullsTab owner={owner} name={name} role={repo?.role} />
            </Suspense>
          </TabsContent>

          <TabsContent value="pipeline">
            <Suspense fallback={<TabFallback />}>
              <PipelineTab owner={owner} name={name} role={repo?.role} />
            </Suspense>
          </TabsContent>

          <TabsContent value="copilot">
            <Suspense fallback={<TabFallback />}>
              <CopilotTab owner={owner} name={name} role={repo?.role} />
            </Suspense>
          </TabsContent>

          <TabsContent value="releases">
            <Suspense fallback={<TabFallback />}>
              <ReleasesTab owner={owner} name={name} role={repo?.role} />
            </Suspense>
          </TabsContent>

        <TabsContent value="projects">
          <Suspense fallback={<TabFallback />}>
            <ProjectsTab owner={owner} name={name} role={repo?.role} />
          </Suspense>
        </TabsContent>

        {isOwner && (
          <TabsContent value="settings">
            <Suspense fallback={<TabFallback />}>
              <SettingsTab owner={owner} name={name} repo={repo} setRepo={setRepo} />
            </Suspense>
          </TabsContent>
        )}
      </Tabs>

      {fileOp && (
        <FileOpDialog
          open
          onOpenChange={(o) => !o && setFileOp(null)}
          owner={owner}
          repo={name}
          branches={branches.map((b) => b.name)}
          init={fileOp}
          onSaved={afterCommit}
        />
      )}
      <RefsDialog
        open={refsOpen}
        onClose={() => setRefsOpen(false)}
        owner={owner}
        repo={name}
        branches={branches.map((b) => b.name)}
        head={branches.find((b) => b.is_head)?.name ?? branches[0]?.name ?? ""}
        tags={tags}
        current={ref || branches[0]?.name || "main"}
        onRefresh={refreshRefs}
      />
      <Dialog open={forkOpen} onOpenChange={setForkOpen}>
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("social.forkTitle")}</DialogTitle>
            <DialogDescription>
              {t("social.forkDescription", { name: `${owner}/${name}` })}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="fork-name">{t("social.forkNameLabel")}</Label>
            <Input
              id="fork-name"
              value={forkName}
              onChange={(e) => setForkName(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button onClick={doFork} disabled={forkBusy || !forkName.trim()}>
              {forkBusy ? t("social.forking") : t("social.fork")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <MirrorDialog
        open={mirrorOpen}
        onOpenChange={setMirrorOpen}
        owner={owner}
        repo={name}
      />
      <ConfirmDialog
        open={pendingRemove !== null}
        onOpenChange={(o) => !o && setPendingRemove(null)}
        description={
          pendingRemove?.isDir
            ? t("fops.confirmDeleteFolder", { path: pendingRemove.path })
            : t("fops.confirmDeleteFile", { path: pendingRemove?.path ?? "" })
        }
        onConfirm={confirmRemove}
      />
    </div>
  );
}

function TabFallback() {
  return (
    <div className="space-y-3 pt-2">
      <Skeleton className="h-9 w-48" />
      <Skeleton className="h-40 w-full" />
    </div>
  );
}
