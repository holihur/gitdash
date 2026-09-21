import { ChevronRight, FileText, Folder, Pencil, TextCursorInput, Trash2 } from "lucide-react";
import type { TreeEntry } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { CommitMessage } from "@/components/commit-message";
import { cn, formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { RelativeTime } from "@/components/relative-time";

interface Props {
  entries: TreeEntry[];
  currentDir: string;
  locale: string;
  onOpenEntry: (entry: TreeEntry) => void;
  onEditPath: (path: string) => void;
  onRename: (path: string, isDir: boolean) => void;
  onRemove: (path: string, isDir: boolean) => void;
}

/** 目录内容表格：名称 / 大小 / 最后提交 / 作者 / 时间，行内提供编辑与删除。 */
export default function TreeListing({
  entries,
  currentDir,
  locale,
  onOpenEntry,
  onEditPath,
  onRename,
  onRemove,
}: Props) {
  const { t } = useI18n();
  return (
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
                        onClick={() => onOpenEntry(entry)}
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
                            <DropdownMenuItem onClick={() => onEditPath(targetPath)}>
                              <Pencil className="h-3.5 w-3.5" />
                              {t("fops.editFile")}
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuItem onClick={() => onRename(targetPath, entry.type === "tree")}>
                            <TextCursorInput className="h-3.5 w-3.5" />
                            {t("fops.rename")}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onClick={() => onRemove(targetPath, entry.type === "tree")}
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
                  <TableCell className="w-72 max-w-[18rem] text-sm text-muted-foreground">
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
                        <CommitMessage message={entry.modified_msg} />
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
                    {entry.modified_at ? <RelativeTime iso={entry.modified_at} locale={locale} /> : "-"}
                  </TableCell>
                </TableRow>
              );
            })}
          {entries.filter((e) => e.name !== ".gitkeep").length === 0 && (
            <TableRow>
              <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                {t("repo.emptyDir")}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
