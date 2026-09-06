import {
  ChevronDown,
  ChevronRight,
  Copy,
  FilePlus2,
  FileText,
  Folder,
  FolderPlus,
  GitBranch,
  GitBranchPlus,
  Pencil,
  Tag as TagIcon,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { api, type Blame, type Blob, type Branch, type Tag, type TreeEntry } from "@/lib/api";
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
import { MarkdownView } from "@/components/markdown";
import CodeMirrorEditor from "@/components/code-editor-lazy";

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
  blob: Blob | null;
  blame: Blame | null;
  error: string;
  emptyRepo: boolean;
  readmeContent: string | null;
  readmeEntryName: string | null;
  blameParam: boolean;
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
  blob,
  blame,
  error,
  emptyRepo,
  readmeContent,
  readmeEntryName,
  blameParam,
  setParams,
  commands,
  openRefs,
  openCreateDialog,
  openEditDialog,
  removeEntry,
  copy,
}: CodeTabProps) {
  const { t, to } = useI18n();

  const openEntry = (entry: TreeEntry) => {
    if (entry.type === "tree") {
      setParams({ path: [...path, entry.name].join("/") });
      return;
    }
    setParams({ file: [...path, entry.name].join("/") });
  };

  const jumpTo = (depth: number) => {
    const next = depth < 0 ? [] : path.slice(0, depth + 1);
    setParams({ path: next.length ? next.join("/") : null });
  };

  const readmeEntry = readmeEntryName ? { name: readmeEntryName } : null;

  return (
    <div className="space-y-4">
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
                      <div className="ml-auto flex shrink-0 items-center gap-0.5">
                        {entry.type === "blob" && (
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            title={t("fops.editFile")}
                            onClick={() =>
                              api
                                .blob(owner, name, refName, targetPath)
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
    </div>
  );
}
