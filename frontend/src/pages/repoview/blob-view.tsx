import type { Ref } from "react";
import { History, MoreHorizontal, Pencil, Trash2 } from "lucide-react";
import type { Blame, Blob } from "@/lib/api";
import type { RepoLinkTarget } from "@/lib/md-links";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { MarkdownView } from "@/components/markdown";
import CodeMirrorEditor from "@/components/code-editor-lazy";
import { formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

function isMarkdown(path: string): boolean {
  const base = path.split("/").pop() ?? "";
  const lower = base.toLowerCase();
  return /^readme(\.(md|markdown|txt))?$/.test(lower) || /\.(md|markdown)$/.test(lower);
}

interface Props {
  owner: string;
  name: string;
  refName: string;
  blob: Blob;
  blame: Blame | null;
  blameParam: boolean;
  codeHostRef: Ref<HTMLDivElement>;
  onToggleBlame: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onOpenRepoLink: (target: RepoLinkTarget) => void;
}

/** 单文件内容视图：blame / markdown / 代码高亮，含编辑与删除操作。 */
export default function BlobView({
  owner,
  name,
  refName,
  blob,
  blame,
  blameParam,
  codeHostRef,
  onToggleBlame,
  onEdit,
  onDelete,
  onOpenRepoLink,
}: Props) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-start gap-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <CardTitle className="break-all font-mono text-sm">{blob.path}</CardTitle>
            <Badge variant="secondary" className="shrink-0">{formatSize(blob.size)}</Badge>
            {blob.encoding !== "utf-8" && (
              <Badge variant="destructive" className="shrink-0">
                {blob.encoding === "binary" ? t("repo.binaryFile") : t("repo.fileTooLarge")}
              </Badge>
            )}
          </div>
          {blob.encoding === "utf-8" && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-muted-foreground"
                  aria-label={t("common.moreActions")}
                  title={t("common.moreActions")}
                >
                  <MoreHorizontal className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuCheckboxItem
                  checked={blameParam}
                  onCheckedChange={() => onToggleBlame()}
                >
                  <History className="h-4 w-4" />
                  {t("repo.blame")}
                </DropdownMenuCheckboxItem>
                <DropdownMenuItem onClick={onEdit}>
                  <Pencil className="h-4 w-4" />
                  {t("fops.editFile")}
                </DropdownMenuItem>
                <DropdownMenuItem
                  className="text-destructive focus:text-destructive"
                  onClick={onDelete}
                >
                  <Trash2 className="h-4 w-4" />
                  {t("fops.deleteFile")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
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
                            href={`/repo/${owner}/${name}/commits`}
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
              <MarkdownView
                text={blob.content}
                repo={{ owner, name, ref: refName, path: blob.path }}
                onOpenRepoLink={onOpenRepoLink}
              />
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
  );
}
