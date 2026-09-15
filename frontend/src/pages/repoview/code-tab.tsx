import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  ChevronDown,
  ChevronRight,
  Copy,
  FilePlus2,
  FileText,
  Folder,
  FolderPlus,
  FolderTree,
  GitBranch,
  GitBranchPlus,
  GitCommitHorizontal,
  List,
  Pencil,
  Search,
  Tag as TagIcon,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { api, type Blame, type Blob, type Branch, type Commit, type SearchResult, type Tag, type TreeEntry } from "@/lib/api";
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
import { cn, formatDate, formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { MarkdownView, MarkdownWithToc } from "@/components/markdown";
import CodeMirrorEditor from "@/components/code-editor-lazy";
import FileTree from "@/components/file-tree";
import Outline from "@/components/outline";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";

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

/** 高亮命中子串（大小写不敏感，只高亮第一处） */
function HighlightText({ text, q }: { text: string; q: string }) {
  if (!q) return <>{text}</>;
  const idx = text.toLowerCase().indexOf(q.toLowerCase());
  if (idx < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="rounded-sm bg-yellow-200/80 text-inherit dark:bg-yellow-500/30">
        {text.slice(idx, idx + q.length)}
      </mark>
      {text.slice(idx + q.length)}
    </>
  );
}

/** 代码搜索：防抖 300ms，Enter 立即搜索 */
function CodeSearch({
  owner,
  name,
  refName,
  setParams,
  actions,
}: {
  owner: string;
  name: string;
  refName: string;
  setParams: (patch: Record<string, string | null>) => void;
  /** 与搜索框同行的操作按钮（新建文件/文件夹等） */
  actions?: ReactNode;
}) {
  const { t } = useI18n();
  const [q, setQ] = useState("");
  const [results, setResults] = useState<SearchResult[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [expanded, setExpanded] = useState(true);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const run = useMemo(
    () => async (query: string) => {
      const trimmed = query.trim();
      if (!trimmed) {
        setResults(null);
        return;
      }
      setLoading(true);
      try {
        setResults(await api.searchRepo(owner, name, trimmed, refName || undefined));
      } catch (e) {
        toast.error(e instanceof Error ? e.message : String(e));
        setResults([]);
      } finally {
        setLoading(false);
      }
    },
    [owner, name, refName],
  );

  const onChange = (value: string) => {
    setQ(value);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => run(value), 300);
  };

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-0 flex-1 basis-52">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={q}
            onChange={(e) => onChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                if (timer.current) clearTimeout(timer.current);
                run(q);
              }
            }}
            placeholder={t("search.placeholder")}
            className="h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        {actions}
      </div>
      {results !== null && (
        <div className="rounded-lg border bg-card">
          <button
            type="button"
            className="flex w-full items-center gap-2 border-b px-3 py-2 text-xs text-muted-foreground"
            onClick={() => setExpanded((v) => !v)}
          >
            {loading ? "…" : t("search.count", { count: results.length })}
            <span className="ml-auto">{expanded ? "▾" : "▸"}</span>
          </button>
          {expanded && (
            <div className="max-h-72 overflow-auto p-1">
              {results.length === 0 && !loading && (
                <p className="px-3 py-2 text-xs text-muted-foreground">{t("search.empty")}</p>
              )}
              {results.map((r, i) => (
                <button
                  key={`${r.path}:${r.line}:${i}`}
                  className="block w-full rounded px-2 py-1 text-left text-xs hover:bg-muted"
                  onClick={() => setParams({ file: r.path, line: String(r.line) })}
                  title={`${r.path}:${r.line}`}
                >
                  <span className="mr-2 font-mono font-medium">{r.path}:{r.line}</span>
                  <span className="font-mono text-muted-foreground">
                    <HighlightText text={r.text} q={q.trim()} />
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export interface CodeTabProps {
  owner: string;
  name: string;
  refName: string;
  locale: string;
  path: string[];
  currentDir: string;
  branches: Branch[];
  tags: Tag[];
  entries: TreeEntry[];
  dirLatestCommit?: Commit | null;
  blob: Blob | null;
  blame: Blame | null;
  error: string;
  emptyRepo: boolean;
  readmeContent: string | null;
  readmeEntryName: string | null;
  blameParam: boolean;
  lineParam: number | null;
  setParams: (patch: Record<string, string | null>) => void;
  commands: string[];
  openRefs: () => void;
  openCreateDialog: (kind: "create-file" | "create-dir") => void;
  openEditDialog: (filePath: string, content: string) => void;
  removeEntry: (targetPath: string, isDir: boolean) => void;
  copy: (text: string) => void;
}

export default function CodeTab({
  owner,
  name,
  refName,
  locale,
  path,
  currentDir,
  branches,
  tags,
  entries,
  dirLatestCommit,
  blob,
  blame,
  error,
  emptyRepo,
  readmeContent,
  readmeEntryName,
  blameParam,
  lineParam,
  setParams,
  commands,
  openRefs,
  openCreateDialog,
  openEditDialog,
  removeEntry,
  copy,
}: CodeTabProps) {
  const { t, to } = useI18n();
  // 窄屏下文件树 / 大纲以左右抽屉形式呈现
  const [treeOpen, setTreeOpen] = useState(false);
  const [outlineOpen, setOutlineOpen] = useState(false);

  const openEntry = (entry: TreeEntry) => {
    if (entry.type === "tree") {
      setParams({ path: [...path, entry.name].join("/"), file: null, line: null, blame: null });
      return;
    }
    setParams({ file: [...path, entry.name].join("/"), line: null, blame: null });
  };

  const openDir = (dir: string) => {
    setParams({ path: dir || null, file: null, line: null, blame: null });
  };

  const openFile = (file: string) => {
    setParams({ file, line: null, blame: null });
  };

  const jumpToId = (id: string) => {
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  const jumpToLine = (line: number) => {
    setParams({ line: String(line) });
  };

  // 大纲内容仅在文本文件（非 blame）时才有意义
  const showOutline = !emptyRepo && !blameParam && !!blob && blob.encoding === "utf-8";
  // 目录列表默认不展示侧栏；打开文件后才显示左侧文件树（大纲同样需打开文件）
  const showTree = !emptyRepo && !!blob;
  // “综合”最后提交：打开文件时用文件的，否则用当前目录的
  const latestCommit = blob ? blob.latest_commit : dirLatestCommit;

  // 回到目录列表（未打开文件）时关闭窄屏抽屉，避免残留
  useEffect(() => {
    if (!showTree) {
      setTreeOpen(false);
      setOutlineOpen(false);
    }
  }, [showTree]);

  // 抽屉内的跳转：操作后自动收起，避免遮挡内容
  const openDirFromTree = (dir: string) => {
    setTreeOpen(false);
    openDir(dir);
  };
  const openFileFromTree = (file: string) => {
    setTreeOpen(false);
    openFile(file);
  };
  const jumpFromOutlineId = (id: string) => {
    setOutlineOpen(false);
    jumpToId(id);
  };
  const jumpFromOutlineLine = (line: number) => {
    setOutlineOpen(false);
    jumpToLine(line);
  };

  const readmeEntry = readmeEntryName ? { name: readmeEntryName } : null;

  // 面包屑：打开文件时按文件完整路径显示，避免与当前目录不同步
  const crumbs = blob ? blob.path.split("/") : path;

  // 搜索结果带行号跳转：滚动到目标行并短暂高亮
  const codeHostRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (lineParam == null || !codeHostRef.current) return;
    const el = codeHostRef.current.querySelector<HTMLElement>(
      `.cm-line:nth-of-type(${lineParam})`,
    );
    if (el) {
      el.scrollIntoView({ block: "center" });
      el.style.backgroundColor = "rgb(250 204 21 / 0.35)";
      const t = setTimeout(() => {
        el.style.backgroundColor = "";
      }, 2000);
      return () => clearTimeout(t);
    }
  }, [lineParam, blob]);

  // 搜索框同行的操作区：窄屏的文件树/大纲按钮 + 新建文件/文件夹
  const toolbarActions = (
    <>
      {showTree && (
        <Button
          size="sm"
          variant="outline"
          className="gap-1.5 lg:hidden"
          onClick={() => setTreeOpen(true)}
        >
          <FolderTree className="h-4 w-4" />
          {t("repo.files")}
        </Button>
      )}
      {showOutline && (
        <Button
          size="sm"
          variant="outline"
          className="gap-1.5 lg:hidden"
          onClick={() => setOutlineOpen(true)}
        >
          <List className="h-4 w-4" />
          {t("outline.title")}
        </Button>
      )}
      <Button size="sm" variant="outline" className="gap-1.5" onClick={() => openCreateDialog("create-file")}>
        <FilePlus2 className="h-4 w-4" />
        {t("fops.newFile")}
      </Button>
      <Button size="sm" variant="outline" className="gap-1.5" onClick={() => openCreateDialog("create-dir")}>
        <FolderPlus className="h-4 w-4" />
        {t("fops.newFolder")}
      </Button>
    </>
  );

  return (
    <div className="space-y-4">
      {!emptyRepo ? (
        <CodeSearch
          owner={owner}
          name={name}
          refName={refName}
          setParams={setParams}
          actions={toolbarActions}
        />
      ) : (
        <div className="flex flex-wrap items-center justify-end gap-2">{toolbarActions}</div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="max-w-full gap-2" disabled={emptyRepo}>
              <GitBranch className="h-4 w-4 shrink-0" />
              <span className="truncate">{refName || t("repo.noBranch")}</span>
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
          onClick={() => openRefs()}
        >
          <GitBranchPlus className="h-4 w-4" />
        </Button>

        {!emptyRepo && (
          <nav className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
            <button className="font-medium hover:underline" onClick={() => openDir("")}>
              {name}
            </button>
            {crumbs.map((seg, i) => {
              const isFile = blob != null && i === crumbs.length - 1;
              return (
                <span key={`${seg}-${i}`} className="flex items-center gap-1">
                  <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                  {isFile ? (
                    <span className="font-medium">{seg}</span>
                  ) : (
                    <button
                      className={cn(
                        "hover:underline",
                        i === crumbs.length - 1 ? "font-medium" : "text-muted-foreground",
                      )}
                      onClick={() => openDir(crumbs.slice(0, i + 1).join("/"))}
                    >
                      {seg}
                    </button>
                  )}
                </span>
              );
            })}
          </nav>
        )}
      </div>

      <div
        className={cn(
          "grid gap-4",
          showTree &&
            (showOutline
              ? "lg:grid-cols-[220px_minmax(0,1fr)_220px] xl:grid-cols-[230px_minmax(0,1fr)_250px]"
              : "lg:grid-cols-[230px_minmax(0,1fr)]"),
        )}
      >
      {showTree && (
        <aside className="sticky top-20 hidden self-start rounded-lg border bg-card lg:block">
          <div className="flex items-center gap-2 border-b px-3 py-2">
            <FolderTree className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <span className="text-xs font-medium text-muted-foreground">{t("repo.files")}</span>
          </div>
          <div className="max-h-[calc(100vh-14rem)] overflow-auto">
            <FileTree
              owner={owner}
              name={name}
              refName={refName}
              currentDir={currentDir}
              activeFile={blob?.path ?? ""}
              emptyRepo={emptyRepo}
              onOpenDir={openDir}
              onOpenFile={openFile}
            />
          </div>
        </aside>
      )}

      <div className="min-w-0 space-y-4">

      {!emptyRepo && !error && latestCommit && (
        <div className="flex items-center gap-3 rounded-lg border bg-card px-3 py-2 text-sm">
          <GitCommitHorizontal className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate font-medium" title={latestCommit.message}>
            {latestCommit.message}
          </span>
          <span className="hidden shrink-0 text-muted-foreground sm:inline">
            {latestCommit.author}
          </span>
          <code
            className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs"
            title={latestCommit.sha}
          >
            {latestCommit.sha.slice(0, 7)}
          </code>
          <span className="shrink-0 whitespace-nowrap text-muted-foreground">
            {formatDate(latestCommit.date, locale)}
          </span>
        </div>
      )}

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
                  <div ref={codeHostRef} className="h-[65vh] overflow-auto bg-background/60">
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
          <Table className="min-w-[860px]">
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead className="w-28 text-right">{t("common.size")}</TableHead>
                <TableHead className="w-72">{t("fops.lastCommit")}</TableHead>
                <TableHead className="w-32">{t("common.author")}</TableHead>
                <TableHead className="w-40 whitespace-nowrap">{t("fops.lastCommitTime")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries
                .filter((e) => e.name !== ".gitkeep")
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
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground">
                            <ChevronRight className="h-3.5 w-3.5 rotate-90" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          {entry.type === "blob" && (
                            <DropdownMenuItem
                              onClick={() =>
                                api
                                  .blob(owner, name, refName, targetPath)
                                  .then((b) => b.encoding === "utf-8" && openEditDialog(targetPath, b.content))
                                  .catch((e) => toast.error(apiErrorMsg(to, e)))
                              }
                            >
                              <Pencil className="h-3.5 w-3.5" />
                              {t("fops.editFile")}
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onClick={() => removeEntry(targetPath, entry.type === "tree")}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                            {entry.type === "tree" ? t("fops.deleteFolder") : t("fops.deleteFile")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                  <TableCell className="text-right text-sm text-muted-foreground">
                    {entry.type === "blob" ? formatSize(entry.size) : "-"}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {entry.last_commit || entry.modified_msg ? (
                      <div className="flex min-w-0 items-center gap-2">
                        {entry.last_commit && (
                          <code
                            className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs"
                            title={entry.last_commit}
                          >
                            {entry.last_commit.slice(0, 7)}
                          </code>
                        )}
                        <span className="truncate" title={entry.modified_msg || undefined}>
                          {entry.modified_msg}
                        </span>
                      </div>
                    ) : (
                      "-"
                    )}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    <span className="block truncate" title={entry.modified_by || undefined}>
                      {entry.modified_by || "-"}
                    </span>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                    {entry.modified_at ? formatDate(entry.modified_at, locale) : "-"}
                  </TableCell>
                </TableRow>
                );
                })}
              {entries.filter((e) => e.name !== ".gitkeep").length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={5}
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
          <div className="p-4">
            <MarkdownWithToc text={readmeContent} />
          </div>
        </div>
      )}
      </div>

      {showOutline && blob && (
        <aside className="sticky top-20 hidden self-start rounded-lg border bg-card lg:block">
          <div className="flex items-center gap-2 border-b px-3 py-2">
            <List className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <span className="text-xs font-medium text-muted-foreground">{t("outline.title")}</span>
          </div>
          <div className="max-h-[calc(100vh-14rem)] overflow-auto p-1">
            <Outline
              path={blob.path}
              content={blob.content}
              onJumpToId={jumpToId}
              onJumpToLine={jumpToLine}
            />
          </div>
        </aside>
      )}
      </div>

      {/* 窄屏：左侧文件树抽屉 */}
      <Sheet open={treeOpen} onOpenChange={setTreeOpen}>
        <SheetContent side="left" aria-describedby={undefined} className="w-72 gap-0 p-0">
          <SheetHeader className="border-b px-3 py-2 pr-10">
            <SheetTitle className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
              <FolderTree className="h-3.5 w-3.5 shrink-0" />
              {t("repo.files")}
            </SheetTitle>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-auto">
            <FileTree
              owner={owner}
              name={name}
              refName={refName}
              currentDir={currentDir}
              activeFile={blob?.path ?? ""}
              emptyRepo={emptyRepo}
              onOpenDir={openDirFromTree}
              onOpenFile={openFileFromTree}
            />
          </div>
        </SheetContent>
      </Sheet>

      {/* 窄屏：右侧大纲抽屉 */}
      {showOutline && blob && (
        <Sheet open={outlineOpen} onOpenChange={setOutlineOpen}>
          <SheetContent side="right" aria-describedby={undefined} className="w-72 gap-0 p-0">
            <SheetHeader className="border-b px-3 py-2 pr-10">
              <SheetTitle className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                <List className="h-3.5 w-3.5 shrink-0" />
                {t("outline.title")}
              </SheetTitle>
            </SheetHeader>
            <div className="min-h-0 flex-1 overflow-auto p-1">
              <Outline
                path={blob.path}
                content={blob.content}
                onJumpToId={jumpFromOutlineId}
                onJumpToLine={jumpFromOutlineLine}
              />
            </div>
          </SheetContent>
        </Sheet>
      )}
    </div>
  );
}
