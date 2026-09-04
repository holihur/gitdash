import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  BadgeCheck,
  ChevronDown,
  ChevronRight,
  Copy,
  FilePlus2,
  FileText,
  Folder,
  FolderPlus,
  GitBranch,
  GitBranchPlus,
  GitCommitHorizontal,
  Tag as TagIcon,
  MoreVertical,
  Pencil,
  Trash2,
} from "lucide-react";
import { api, cloneCommand, type Blob, type Branch, type Commit, type PullDiff, type Repo, type Tag, type TreeEntry } from "@/lib/api";
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn, formatDate, formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { MarkdownView } from "@/components/markdown";
import { DiffView, type DiffFileInfo } from "@/components/diff-view";
import CodeMirrorEditor from "@/components/code-editor";
import FileOpDialog, { type FileOp } from "@/components/file-op-dialog";
import RefsDialog from "@/components/refs-dialog";
import RepoIssues from "@/pages/RepoIssues";
import RepoPulls from "@/pages/RepoPulls";

// 判断文件名是否需要 Markdown 渲染（README 任意扩展名或 .md/.markdown）
function isMarkdown(path: string): boolean {
  const base = path.split("/").pop() ?? "";
  const lower = base.toLowerCase();
  return /^readme(\.(md|markdown|txt))?$/.test(lower) || /\.(md|markdown)$/.test(lower);
}

function CodeBlock({ text, onCopy }: { text: string; onCopy: () => void }) {
  return (
    <div className="flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-2">
      <code className="flex-1 overflow-x-auto whitespace-pre text-xs">{text}</code>
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
  const [repo, setRepo] = useState<Repo | null>(null);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [ref, setRef] = useState("");
  const [path, setPath] = useState<string[]>([]);
  const [entries, setEntries] = useState<TreeEntry[]>([]);
  const [blob, setBlob] = useState<Blob | null>(null);
  const [commits, setCommits] = useState<Commit[]>([]);
  const [diffSha, setDiffSha] = useState<string | null>(null);
  const [diffData, setDiffData] = useState<{ files: DiffFileInfo[]; patch: string } | null>(null);
  const [tab, setTab] = useState("code");
  const [error, setError] = useState("");
  const [missing, setMissing] = useState(false);
  const [fileOp, setFileOp] = useState<FileOp | null>(null);
  const [loadTick, setLoadTick] = useState(0);
  const [tags, setTags] = useState<Tag[]>([]);
  const [refsOpen, setRefsOpen] = useState(false);
  const currentDir = path.join("/");
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
      navigator.clipboard.writeText(text).then(() => toast.success(t("common.copied")));
    },
    [t],
  );

  useEffect(() => {
    (async () => {
      try {
        const [r, bs] = await Promise.all([api.getRepo(owner, name), api.branches(owner, name)]);
        setRepo(r);
        setBranches(bs);
        const head = bs.find((b) => b.is_head) ?? bs[0];
        if (head) setRef(head.name);
        api.listTags(owner, name).then(setTags).catch(() => undefined);
      } catch (e) {
        setMissing(true);
        setError(e instanceof Error ? e.message : String(e));
      }
    })();
  }, [name]);

  const loadTree = useCallback(async () => {
    void loadTick;
    if (!ref) return;
    try {
      const dir = path.join("/");
      const data = await api.tree(owner, name, ref, dir);
      setEntries(data.entries);
      setBlob(null);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [name, ref, path, loadTick]);

  useEffect(() => {
    loadTree();
  }, [loadTree]);

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

  const openEntry = async (entry: TreeEntry) => {
    if (entry.type === "tree") {
      setPath([...path, entry.name]);
      return;
    }
    try {
      setBlob(await api.blob(owner, name, ref, [...path, entry.name].join("/")));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const jumpTo = (depth: number) => {
    // depth = -1 -> root, otherwise index of segment
    setPath(depth < 0 ? [] : path.slice(0, depth + 1));
    setBlob(null);
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

  const removeEntry = async (targetPath: string, isDir: boolean) => {
    const ok = window.confirm(
      isDir
        ? t("fops.confirmDeleteFolder", { path: targetPath })
        : t("fops.confirmDeleteFile", { path: targetPath }),
    );
    if (!ok) return;
    const branch = ref || branches[0]?.name || "main";
    try {
      await api.createCommit(owner, name, branch, `Delete ${targetPath}`, [
        { path: targetPath, action: isDir ? "delete_tree" : "delete" },
      ]);
      toast.success(t("fops.deleted", { path: targetPath }));
      afterCommit(branch, isDir ? currentDir : undefined);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const afterCommit = async (branch: string, backToDir?: string) => {
    setBlob(null);
    if (backToDir !== undefined) setPath(backToDir ? backToDir.split("/") : []);
    else if (blob) setPath([]);
    setRef(branch);
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

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-bold">
            {owner}/{name}
          </h1>
          <p className="text-sm text-muted-foreground">{repo?.description || t("common.noDescription")}</p>
        </div>
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

      <Tabs value={tab} onValueChange={setTab}>
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
                    onClick={() => {
                      setRef(b.name);
                      setPath([]);
                      setBlob(null);
                    }}
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
                        onClick={() => {
                          setRef(tg.name);
                          setPath([]);
                          setBlob(null);
                        }}
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
                {blob.encoding === "utf-8" ? (
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
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((entry) => {
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
                    </TableRow>
                    );
                  })}
                  {entries.length === 0 && (
                    <TableRow>
                      <TableCell
                        colSpan={3}
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
    </div>
  );
}
