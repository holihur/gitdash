import { Suspense, lazy, useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { api, cloneCommand } from "@/lib/api";
import { buildFileOpPath, type RepoTab } from "@/lib/repo-url";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { TabsListOverflow } from "@/components/ui/tabs-overflow";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { copyText } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import RefsDialog from "@/components/refs-dialog";
import RepoHeader from "./repoview/repo-header";
import ForkDialog from "./repoview/fork-dialog";
import { useRepoRouting } from "./repoview/use-repo-routing";
import { useRepoData } from "./repoview/use-repo-data";

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
  const navigate = useNavigate();

  const { owner, name, tab, fileParam, path, currentDir, blameParam, urlRef, lineParam, issueNumber, setParams } =
    useRepoRouting();

  const {
    repo,
    setRepo,
    branches,
    entries,
    treeLatestCommit,
    blob,
    blame,
    error,
    missing,
    readmeContent,
    readmeEntryName,
    tags,
    ref,
    refreshRefs,
    refreshBranches,
    reloadTree,
  } = useRepoData({ owner, name, currentDir, fileParam, blameParam, urlRef, setParams });

  // 仓库角色由后端按 owner / 协作者 / 组织角色计算；组织仓库的 owner 也是"owner"，
  // 不能用 me === owner 判断（组织仓库 owner 是组织名）。
  const isOwner = repo?.role === "owner";

  const [refsOpen, setRefsOpen] = useState(false);
  const [starBusy, setStarBusy] = useState(false);
  const [watchBusy, setWatchBusy] = useState(false);
  const [forkOpen, setForkOpen] = useState(false);
  const [forkName, setForkName] = useState("");
  const [forkBusy, setForkBusy] = useState(false);
  const [pendingRemove, setPendingRemove] = useState<{ path: string; isDir: boolean } | null>(null);

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

  // 新建 / 编辑改为独立整页路由，便于刷新、分享与浏览器回退。
  const openCreatePage = (kind: "create-file" | "create-dir") => {
    const params = new URLSearchParams();
    params.set("kind", kind === "create-dir" ? "dir" : "file");
    if (currentDir) params.set("dir", currentDir);
    if (ref) params.set("ref", ref);
    navigate(`${buildFileOpPath(owner, name, "new")}?${params.toString()}`);
  };

  const openEditPage = (filePath: string) => {
    const params = new URLSearchParams();
    params.set("path", filePath);
    if (ref) params.set("ref", ref);
    navigate(`${buildFileOpPath(owner, name, "edit")}?${params.toString()}`);
  };

  const openRenamePage = (targetPath: string, isDir: boolean) => {
    const params = new URLSearchParams();
    params.set("path", targetPath);
    params.set("kind", isDir ? "dir" : "file");
    if (ref) params.set("ref", ref);
    navigate(`${buildFileOpPath(owner, name, "rename")}?${params.toString()}`);
  };

  const removeEntry = (targetPath: string, isDir: boolean) => {
    setPendingRemove({ path: targetPath, isDir });
  };

  const afterCommit = async (branch: string, backToDir?: string) => {
    const nextPath =
      backToDir !== undefined ? (backToDir ? backToDir.split("/") : []) : blob ? [] : path;
    setParams({
      ref: branch,
      path: nextPath.length ? nextPath.join("/") : null,
      file: null,
    });
    reloadTree();
    await refreshBranches();
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
      <RepoHeader
        owner={owner}
        name={name}
        repo={repo}
        isOwner={isOwner}
        watchBusy={watchBusy}
        starBusy={starBusy}
        onToggleWatch={toggleWatch}
        onToggleStar={toggleStar}
        onFork={openFork}
        onCopy={copy}
      />

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
              repo={repo}
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
              readmeEntryName={readmeEntryName}
              blameParam={blameParam}
              lineParam={lineParam}
              setParams={setParams}
              commands={commands}
              openRefs={() => setRefsOpen(true)}
              openCreateDialog={openCreatePage}
              openEditDialog={openEditPage}
              renameEntry={openRenamePage}
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
            <IssuesTab owner={owner} name={name} role={repo?.role} issueNumber={issueNumber} />
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

      <RefsDialog
        open={refsOpen}
        onClose={() => setRefsOpen(false)}
        owner={owner}
        repo={name}
        current={ref || branches[0]?.name || "main"}
        canWrite={repo?.role === "owner" || repo?.role === "write"}
        onRefresh={refreshRefs}
      />
      <ForkDialog
        open={forkOpen}
        onOpenChange={setForkOpen}
        owner={owner}
        name={name}
        forkName={forkName}
        onForkName={setForkName}
        busy={forkBusy}
        onFork={doFork}
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
