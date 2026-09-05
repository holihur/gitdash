import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import {
  BadgeCheck,
  ChevronDown,
  ChevronRight,
  Copy,
  Eye,
  FilePlus2,
  FileText,
  Folder,
  FolderPlus,
  GitBranch,
  GitBranchPlus,
  GitCommitHorizontal,
  GitFork,
  Star,
  Tag as TagIcon,
  MoreVertical,
  Pencil,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { api, cloneCommand, type Blame, type Blob, type Branch, type Commit, type PullDiff, type Repo, type Tag, type TreeEntry } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { cn, copyText, formatDate, formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { MarkdownView } from "@/components/markdown";
import { DiffView, type DiffFileInfo } from "@/components/diff-view";
import CodeMirrorEditor from "@/components/code-editor-lazy";
import FileOpDialog, { type FileOp } from "@/components/file-op-dialog";
import MirrorDialog from "@/components/mirror-dialog";
import RefsDialog from "@/components/refs-dialog";
import RepoIssues from "@/pages/RepoIssues";
import RepoPulls from "@/pages/RepoPulls";
import RepoPipeline from "@/pages/RepoPipeline";

// 判断文件名是否需要 Markdown 渲染（README 任意扩展名或 .md/.markdown）
function isMarkdown(path: string): boolean {
  const base = path.split("/").pop() ?? "";
  const lower = base.toLowerCase();
  return /^readme(\.(md|markdown|txt))?$/.test(lower) || /\.(md|markdown)$/.test(lower);
}

function CodeBlock({ text, onCopy }: { text: string; onCopy: () => void }) {
  return (
    <div className="flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-2">
      <code className="min-w-0 flex-1 overflow-x-auto whitespace-pre text-xs">{text}</code>
      <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" onClick={onCopy}>
        <Copy className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

export default function RepoView() {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const { owner = "", name = "" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const tabs = ["code", "commits", "issues", "pulls", "pipeline", "settings"] as const;
  type RepoTab = (typeof tabs)[number];

  // 把 tab / ref / path / file 写进 URL 查询参数，支持刷新与分享
  const setParams = useCallback(
    (patch: Record<string, string | null>) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v === null) next.delete(k);
          else next.set(k, v);
        }
        return next;
      });
    },
    [setSearchParams],
  );

  const [me, setMe] = useState("");
  const [visibilityBusy, setVisibilityBusy] = useState(false);
  const [deleteRepoOpen, setDeleteRepoOpen] = useState(false);
  const [deleteRepoBusy, setDeleteRepoBusy] = useState(false);
  const isOwner = me !== "" && me === owner;

  // 当前登录用户（用于判断仓库归属，决定是否展示设置页）
  useEffect(() => {
    api
      .me()
      .then((m) => setMe(m.username))
      .catch(() => setMe(""));
  }, []);

  const toggleVisibility = async () => {
    if (!repo) return;
    setVisibilityBusy(true);
    try {
      const r = await api.setRepoVisibility(owner, name, !repo.private);
      setRepo({ ...repo, private: r.private });
      toast.success(t(r.private ? "repo.visibilityNowPrivate" : "repo.visibilityNowPublic"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setVisibilityBusy(false);
    }
  };

  const doDeleteRepo = async () => {
    setDeleteRepoBusy(true);
    try {
      await api.deleteRepo(owner, name);
      toast.success(t("repo.repoDeleted"));
      setDeleteRepoOpen(false);
      navigate("/repos");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setDeleteRepoBusy(false);
    }
  };

  const [repo, setRepo] = useState<Repo | null>(null);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [entries, setEntries] = useState<TreeEntry[]>([]);
  const [blob, setBlob] = useState<Blob | null>(null);
  const [blame, setBlame] = useState<Blame | null>(null);
  const [commits, setCommits] = useState<Commit[]>([]);
  const [diffSha, setDiffSha] = useState<string | null>(null);
  const [diffData, setDiffData] = useState<{ files: DiffFileInfo[]; patch: string } | null>(null);
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
  const navigate = useNavigate();

  // URL 参数派生：tab / ref / path / file（blob）
  const tabParam = searchParams.get("tab");
  const tab: RepoTab = (tabs as readonly string[]).includes(tabParam ?? "")
    ? (tabParam as RepoTab)
    : "code";
  const pathParam = searchParams.get("path") ?? "";
  const path = pathParam ? pathParam.split("/") : [];
  const currentDir = path.join("/");
  const fileParam = searchParams.get("file") ?? "";
  const blameParam = searchParams.get("blame") === "1";
  const urlRef = searchParams.get("ref") ?? "";
  const ref =
    urlRef ||
    branches.find((b) => b.is_head)?.name ||
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
      setBlob(null);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [name, ref, currentDir, loadTick]);

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
      .catch((e) => {
        if (!alive) return;
        setBlob(null);
        setParams({ file: null });
        toast.error(apiErrorMsg(to, e));
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

  useEffect(() => {
    if (tab !== "commits" || !ref) return;
    api
      .commits(owner, name, ref)
      .then(setCommits)
      .catch((e) => toast.error(apiErrorMsg(to, e)));
  }, [tab, name, ref]);

  useEffect(() => {
    if (!diffSha) {
      setDiffData(null);
      return;
    }
    let alive = true;
    api
      .commitDiff(owner, name, diffSha)
      .then((d: PullDiff) => alive && setDiffData({ files: d.files, patch: d.patch }))
      .catch(() => alive && setDiffData(null));
    return () => {
      alive = false;
    };
  }, [diffSha, owner, name]);

  const openEntry = (entry: TreeEntry) => {
    if (entry.type === "tree") {
      setParams({ path: [...path, entry.name].join("/") });
      return;
    }
    setParams({ file: [...path, entry.name].join("/") });
  };

  const jumpTo = (depth: number) => {
    // depth = -1 -> root, otherwise index of segment
    const next = depth < 0 ? [] : path.slice(0, depth + 1);
    setParams({ path: next.length ? next.join("/") : null });
  };
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
    [name, branches],
  );

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
            {owner}/{name}
          </h1>
          <p className="text-sm text-muted-foreground">{repo?.description || t("common.noDescription")}</p>
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
          <Button variant="outline" size="sm" className="gap-1.5" onClick={openFork}>
            <GitFork className="h-4 w-4" />
            {t("social.fork")}
          </Button>
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

      <Tabs value={tab} onValueChange={(v) => setParams({ tab: v === "code" ? null : v })}>
        <TabsList className="w-full sm:w-auto">
          <TabsTrigger value="code" className="flex-1 sm:flex-none">
            {t("repo.code")}
          </TabsTrigger>
          <TabsTrigger value="commits" className="flex-1 sm:flex-none">
            {t("repo.commits")}
          </TabsTrigger>
          <TabsTrigger value="issues" className="flex-1 sm:flex-none">
            {t("issues.title")}
          </TabsTrigger>
          <TabsTrigger value="pulls" className="flex-1 sm:flex-none">
            {t("pulls.title")}
          </TabsTrigger>
          <TabsTrigger value="pipeline" className="flex-1 sm:flex-none">
            {t("pipeline.tab")}
          </TabsTrigger>
          {isOwner && (
            <TabsTrigger value="settings" className="flex-1 sm:flex-none">
              {t("repo.settings")}
            </TabsTrigger>
          )}
        </TabsList>

        <TabsContent value="code" className="space-y-4">
          <div className="flex items-center justify-end gap-2">
            <Button size="sm" variant="outline" className="gap-1.5" onClick={() => openCreateDialog("create-file")}>
              <FilePlus2 className="h-4 w-4" />
              {t("fops.newFile")}
            </Button>
            <Button size="sm" variant="outline" className="gap-1.5" onClick={() => openCreateDialog("create-dir")}>
              <FolderPlus className="h-4 w-4" />
              {t("fops.newFolder")}
            </Button>
          </div>
          {/* branch selector + breadcrumb */}
          <div className="flex flex-wrap items-center gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className="max-w-full gap-2" disabled={emptyRepo}>
                  <GitBranch className="h-4 w-4 shrink-0" />
                  <span className="truncate">{ref || t("repo.noBranch")}</span>
                  <ChevronDown className="h-3.5 w-3.5 shrink-0" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="max-h-72 w-64 overflow-auto">
                {branches.map((b) => (
                  <DropdownMenuItem
                    key={b.name}
                    onClick={() => setParams({ ref: b.name, path: null, file: null })}
                  >
                    <GitBranch className="shrink-0" />
                    <span className="truncate">{b.name}</span>
                    {b.is_head && (
                      <Badge variant="secondary" className="ml-auto shrink-0">
                        HEAD
                      </Badge>
                    )}
                  </DropdownMenuItem>
                ))}
                {tags.length > 0 && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuLabel className="text-xs">{t("refs.tags")}</DropdownMenuLabel>
                    {tags.map((tg) => (
                      <DropdownMenuItem
                        key={tg.name}
                        onClick={() => setParams({ ref: tg.name, path: null, file: null })}
                      >
                        <TagIcon className="shrink-0" />
                        <span className="truncate">{tg.name}</span>
                      </DropdownMenuItem>
                    ))}
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>

            <Button
              variant="ghost"
              size="sm"
              className="h-9 w-9 px-0"
              title={t("refs.manage")}
              onClick={() => setRefsOpen(true)}
            >
              <GitBranchPlus className="h-4 w-4" />
            </Button>

            {!emptyRepo && (
              <nav className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
                <button
                  className={cn(
                    "font-medium hover:underline",
                    path.length === 0 || blob ? "" : "text-muted-foreground",
                  )}
                  onClick={() => jumpTo(-1)}
                >
                  {name}
                </button>
                {path.map((seg, i) => (
                  <span key={i} className="flex items-center gap-1">
                    <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                    <button
                      className={cn(
                        "hover:underline",
                        i === path.length - 1 && !blob ? "font-medium" : "text-muted-foreground",
                      )}
                      onClick={() => jumpTo(i)}
                    >
                      {seg}
                    </button>
                  </span>
                ))}
                {blob && (
                  <span className="flex items-center gap-1">
                    <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="font-medium">{blob.path.split("/").pop()}</span>
                  </span>
                )}
              </nav>
            )}
          </div>

          {error && (
            <Card className="border-destructive">
              <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
            </Card>
          )}

          {emptyRepo && !error && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">{t("repo.emptyRepo")}</CardTitle>
                <CardDescription>{t("repo.emptyRepoHint")}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2">
                {commands.map((cmd) => (
                  <CodeBlock key={cmd} text={cmd} onCopy={() => copy(cmd)} />
                ))}
                <p className="pt-2 text-xs text-muted-foreground">{t("repo.sshHint")}</p>
              </CardContent>
            </Card>
          )}

          {!emptyRepo && !error && blob && (
            <Card>
              <CardHeader className="pb-2">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <CardTitle className="break-all font-mono text-sm">{blob.path}</CardTitle>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">{formatSize(blob.size)}</Badge>
                    {blob.encoding !== "utf-8" && (
                      <Badge variant="destructive">
                        {blob.encoding === "binary" ? t("repo.binaryFile") : t("repo.fileTooLarge")}
                      </Badge>
                    )}
                    {blob.encoding === "utf-8" && (
                      <Button
                        size="sm"
                        variant={blameParam ? "default" : "outline"}
                        onClick={() => setParams({ blame: blameParam ? null : "1" })}
                      >
                        {t("repo.blame")}
                      </Button>
                    )}
                    {blob.encoding === "utf-8" && !isMarkdown(blob.path) && (
                      <>
                        <Button size="sm" variant="outline" className="gap-1.5"
                          onClick={() => openEditDialog(blob.path, blob.content)}>
                          <Pencil className="h-3.5 w-3.5" />
                          {t("fops.editFile")}
                        </Button>
                        <Button size="sm" variant="outline" className="gap-1.5 text-destructive hover:text-destructive"
                          onClick={() => removeEntry(blob.path, false)}>
                          <Trash2 className="h-3.5 w-3.5" />
                          {t("fops.deleteFile")}
                        </Button>
                      </>
                    )}
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                {blob.encoding === "utf-8" && blameParam ? (
                  blame ? (
                    <div className="max-h-[70vh] overflow-auto rounded-md border">
                      <table className="w-full border-collapse font-mono text-xs">
                        <tbody>
                          {blame.lines.map((l) => {
                            const c = blame.commits[l.commit];
                            return (
                              <tr key={l.line} className="border-b border-border/50 last:border-0">
                                <td className="w-40 max-w-40 truncate whitespace-nowrap border-r border-border/50 bg-muted/40 px-2 py-0.5 align-top text-muted-foreground">
                                  <a
                                    className="block truncate hover:underline"
                                    href={`/repo/${owner}/${name}?tab=commits`}
                                    title={c ? `${c.author} · ${c.message}` : l.commit}
                                  >
                                    {c ? c.author : l.commit.slice(0, 7)}
                                  </a>
                                </td>
                                <td className="w-10 whitespace-nowrap px-2 py-0.5 align-top text-right text-muted-foreground">
                                  {l.line}
                                </td>
                                <td className="whitespace-pre-wrap break-all px-2 py-0.5 align-top">
                                  {l.content || " "}
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
                  )
                ) : blob.encoding === "utf-8" ? (
                  isMarkdown(blob.path) ? (
                    <div className="max-h-[70vh] overflow-auto rounded-md border bg-muted/30 p-4">
                      <MarkdownView text={blob.content} />
                    </div>
                  ) : (
                    <div className="overflow-hidden rounded-md border">
                      <div className="h-[65vh] overflow-auto bg-background/60">
                        <CodeMirrorEditor value={blob.content} path={blob.path} readOnly />
                      </div>
                    </div>
                  )
                ) : (
                  <p className="text-sm text-muted-foreground">{t("repo.previewNotAvailable")}</p>
                )}
              </CardContent>
            </Card>
          )}

          {!emptyRepo && !error && !blob && (
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[560px]">
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("common.name")}</TableHead>
                    <TableHead className="w-24">{t("common.type")}</TableHead>
                    <TableHead className="w-28 text-right">{t("common.size")}</TableHead>
                    <TableHead className="w-72">{t("fops.lastCommit")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries
                    .filter((e) => e.name !== ".gitkeep") // 隐藏占位文件
                    .map((entry) => {
                    const targetPath = currentDir ? currentDir + "/" + entry.name : entry.name;
                    return (
                    <TableRow key={entry.name}>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <button
                            className="flex min-w-0 items-center gap-2 hover:underline"
                            onClick={() => openEntry(entry)}
                          >
                            {entry.type === "tree" ? (
                              <Folder className="h-4 w-4 shrink-0 text-blue-500" />
                            ) : (
                              <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
                            )}
                            <span className={cn("truncate", entry.type === "tree" ? "font-medium" : "")}>
                              {entry.name}
                            </span>
                          </button>
                          <div className="ml-auto flex shrink-0 items-center gap-0.5">
                            {entry.type === "blob" && (
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-7 w-7"
                                title={t("fops.editFile")}
                                onClick={() =>
                                  api
                                    .blob(owner, name, ref, targetPath)
                                    .then((b) => b.encoding === "utf-8" && openEditDialog(targetPath, b.content))
                                    .catch((e) => toast.error(apiErrorMsg(to, e)))
                                }
                              >
                                <Pencil className="h-3.5 w-3.5" />
                              </Button>
                            )}
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-7 w-7 text-destructive hover:text-destructive"
                              title={entry.type === "tree" ? t("fops.deleteFolder") : t("fops.deleteFile")}
                              onClick={() => removeEntry(targetPath, entry.type === "tree")}
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {entry.type === "tree" ? t("common.directory") : t("common.file")}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">
                        {entry.type === "blob" ? formatSize(entry.size) : "-"}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {entry.modified_at || entry.last_commit ? (
                          <div className="min-w-0 space-y-1">
                            <div className="flex min-w-0 items-center gap-2">
                              {entry.last_commit && (
                                <code
                                  className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs"
                                  title={entry.last_commit}
                                >
                                  {entry.last_commit.slice(0, 7)}
                                </code>
                              )}
                              <span
                                className="truncate"
                                title={[entry.modified_by, entry.modified_msg].filter(Boolean).join(" · ")}
                              >
                                {entry.modified_msg}
                              </span>
                            </div>
                            <div className="truncate text-xs">
                              {entry.modified_by && <span>{entry.modified_by}</span>}
                              <span className={entry.modified_by ? "ml-2" : ""}>
                                {formatDate(entry.modified_at ?? "", locale)}
                              </span>
                            </div>
                          </div>
                        ) : (
                          "-"
                        )}
                      </TableCell>
                    </TableRow>
                    );
                    })}
                  {entries.filter((e) => e.name !== ".gitkeep").length === 0 && (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className="py-10 text-center text-sm text-muted-foreground"
                      >
                        {t("repo.emptyDir")}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          )}

          {!emptyRepo && !error && !blob && readmeContent !== null && readmeEntry && (
            <div className="rounded-lg border bg-card">
              <div className="flex items-center gap-2 border-b px-4 py-2">
                <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-xs font-medium text-muted-foreground">{readmeEntry.name}</span>
              </div>
              <div className="max-h-[60vh] overflow-auto p-4">
                <MarkdownView text={readmeContent} />
              </div>
            </div>
          )}
        </TabsContent>

        <TabsContent value="commits">
          {emptyRepo ? (
            <p className="py-10 text-center text-sm text-muted-foreground">{t("repo.noCommits")}</p>
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[640px]">
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("repo.commit")}</TableHead>
                    <TableHead className="w-36">{t("common.author")}</TableHead>
                    <TableHead className="w-56">{t("common.date")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {commits.map((c) => (
                    <TableRow key={c.sha}>
                      <TableCell>
                        <button
                          type="button"
                          className="flex w-full items-center gap-3 text-left"
                          onClick={() => setDiffSha(diffSha === c.sha ? null : c.sha)}
                          title={t("commits.viewDiff")}
                        >
                          <GitCommitHorizontal className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <code className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs">
                            {c.sha.slice(0, 7)}
                          </code>
                          <span className="truncate">{c.message}</span>
                          {c.gpg_verified && (
                            <Badge
                              variant="outline"
                              className="shrink-0 gap-1 border-green-600/40 text-green-600"
                              title={t("commits.gpgSigned", { user: c.gpg_verified })}
                            >
                              <BadgeCheck className="h-3 w-3" />
                              {c.gpg_verified}
                            </Badge>
                          )}
                          <ChevronDown
                            className={cn(
                              "h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform",
                              diffSha === c.sha && "rotate-180",
                            )}
                          />
                          </button>
                      </TableCell>
                      <TableCell className="text-sm">{c.author}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {formatDate(c.date, locale)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}

          {diffSha && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="font-mono text-sm">{diffSha.slice(0, 12)}</CardTitle>
              </CardHeader>
              <CardContent>
                {diffData ? (
                  <DiffView files={diffData.files} patch={diffData.patch} />
                ) : (
                  <p className="py-6 text-center text-sm text-muted-foreground">…</p>
                )}
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="issues">
          <RepoIssues owner={owner} name={name} />
        </TabsContent>

        <TabsContent value="pulls">
          <RepoPulls owner={owner} name={name} />
        </TabsContent>

        <TabsContent value="pipeline">
          <RepoPipeline owner={owner} name={name} role={repo?.role} />
        </TabsContent>

        <TabsContent value="settings" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("repo.visibility")}</CardTitle>
              <CardDescription>{t("repo.visibilityDesc")}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap items-center gap-3">
                <Badge variant={repo?.private ? "secondary" : "outline"}>
                  {repo?.private ? t("repo.privateRepo") : t("repo.publicRepo")}
                </Badge>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={visibilityBusy || !repo}
                  onClick={toggleVisibility}
                >
                  {repo?.private ? t("repo.makePublic") : t("repo.makePrivate")}
                </Button>
              </div>
            </CardContent>
          </Card>
          <Card className="border-destructive/40">
            <CardHeader>
              <CardTitle className="text-base text-destructive">{t("repo.dangerZone")}</CardTitle>
              <CardDescription>{t("repo.deleteRepoDesc")}</CardDescription>
            </CardHeader>
            <CardContent>
              <Button
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setDeleteRepoOpen(true)}
              >
                <Trash2 className="h-3.5 w-3.5" />
                {t("repo.deleteRepo")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>
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
      <ConfirmDialog
        open={deleteRepoOpen}
        onOpenChange={setDeleteRepoOpen}
        title={t("repo.deleteRepo")}
        description={t("repo.deleteRepoConfirm", { name: `${owner}/${name}` })}
        onConfirm={doDeleteRepo}
        busy={deleteRepoBusy}
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
              {t("social.fork")}
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
