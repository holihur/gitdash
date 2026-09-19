import type { Ref } from "react";
import { FileText, GitCommitHorizontal } from "lucide-react";
import type { Blame, Blob, Commit, TreeEntry } from "@/lib/api";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { MarkdownWithToc } from "@/components/markdown";
import type { RepoLinkTarget } from "@/lib/md-links";
import TreeListing from "@/components/tree-listing";
import { CodeBlock } from "@/components/code-block";
import BlobView from "./blob-view";

interface Props {
  owner: string;
  name: string;
  refName: string;
  locale: string;
  /** 当前 ref 下的文件 / 目录最后提交 */
  latestCommit?: Commit | null;
  error: string;
  emptyRepo: boolean;
  commands: string[];
  copy: (text: string) => void;
  blob: Blob | null;
  blame: Blame | null;
  blameParam: boolean;
  codeHostRef: Ref<HTMLDivElement>;
  onToggleBlame: () => void;
  onEditBlob: (path: string) => void;
  onDeleteBlob: (path: string) => void;
  onOpenRepoLink: (target: RepoLinkTarget) => void;
  entries: TreeEntry[];
  currentDir: string;
  onOpenEntry: (entry: TreeEntry) => void;
  onEditPath: (targetPath: string) => void;
  onRemoveEntry: (targetPath: string, isDir: boolean) => void;
  readmeContent: string | null;
  readmeEntryName: string | null;
  readmePath: string;
}

/** 代码页主内容：最后提交条 / 错误 / 空仓库引导 / 文件内容 / 目录列表 / README。 */
export function RepoCodeBody({
  owner,
  name,
  refName,
  locale,
  latestCommit,
  error,
  emptyRepo,
  commands,
  copy,
  blob,
  blame,
  blameParam,
  codeHostRef,
  onToggleBlame,
  onEditBlob,
  onDeleteBlob,
  onOpenRepoLink,
  entries,
  currentDir,
  onOpenEntry,
  onEditPath,
  onRemoveEntry,
  readmeContent,
  readmeEntryName,
  readmePath,
}: Props) {
  const { t } = useI18n();
  return (
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
        <BlobView
          owner={owner}
          name={name}
          refName={refName}
          blob={blob}
          blame={blame}
          blameParam={blameParam}
          codeHostRef={codeHostRef}
          onToggleBlame={onToggleBlame}
          onEdit={() => onEditBlob(blob.path)}
          onDelete={() => onDeleteBlob(blob.path)}
          onOpenRepoLink={onOpenRepoLink}
        />
      )}

      {!emptyRepo && !error && !blob && (
        <TreeListing
          entries={entries}
          currentDir={currentDir}
          locale={locale}
          onOpenEntry={onOpenEntry}
          onEditPath={onEditPath}
          onRemove={onRemoveEntry}
        />
      )}

      {!emptyRepo && !error && !blob && readmeContent !== null && readmeEntryName && (
        <div className="rounded-lg border bg-card">
          <div className="flex items-center gap-2 border-b px-4 py-2">
            <FileText className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-xs font-medium text-muted-foreground">{readmeEntryName}</span>
          </div>
          <div className="p-4">
            <MarkdownWithToc
              text={readmeContent}
              repo={{ owner, name, ref: refName, path: readmePath }}
              onOpenRepoLink={onOpenRepoLink}
            />
          </div>
        </div>
      )}
    </div>
  );
}
