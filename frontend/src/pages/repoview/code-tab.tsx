import { useEffect, useRef, useState } from "react";
import { ChevronDown, FilePlus2, FolderPlus, FolderTree, List, Plus } from "lucide-react";
import { type Blame, type Blob, type Branch, type Commit, type Tag, type TreeEntry } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import type { RepoLinkTarget } from "@/lib/md-links";
import FileTree from "@/components/file-tree";
import Outline from "@/components/outline";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import CodeRefBar from "./code-ref-bar";
import { CodeSearch } from "@/components/code-search";
import { RepoCodeBody } from "./repo-code-body";



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
  openEditDialog: (filePath: string) => void;
  renameEntry: (targetPath: string, isDir: boolean) => void;
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
  renameEntry,
  removeEntry,
  copy,
}: CodeTabProps) {
  const { t } = useI18n();
  // 窄屏下文件树 / 大纲以左右抽屉形式呈现
  const [treeOpen, setTreeOpen] = useState(false);
  const [outlineOpen, setOutlineOpen] = useState(false);
  const [compareOpen, setCompareOpen] = useState(false);

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

  // README / Markdown 文件里的仓库内引用：在代码浏览器内跳转，保持当前 ref。
  const openRepoLink = (target: RepoLinkTarget) => {
    if (target.kind === "dir") {
      setParams({ path: target.path, file: null, line: null, blame: null, hash: null });
    } else {
      setParams({ file: target.path, line: null, blame: null, hash: target.hash ?? null });
    }
  };

  // 当前 README 在仓库中的完整路径（相对根），用于解析其相对引用。
  const readmePath = readmeEntryName
    ? currentDir
      ? `${currentDir}/${readmeEntryName}`
      : readmeEntryName
    : "";

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

  // 搜索框同行的操作区：窄屏的文件树/大纲按钮 + “新建”下拉（文件 / 文件夹）
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
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="outline" className="gap-1.5">
            <Plus className="h-4 w-4" />
            {t("fops.new")}
            <ChevronDown className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => openCreateDialog("create-file")}>
            <FilePlus2 className="h-4 w-4" />
            {t("fops.newFile")}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => openCreateDialog("create-dir")}>
            <FolderPlus className="h-4 w-4" />
            {t("fops.newFolder")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
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

      <CodeRefBar
        owner={owner}
        name={name}
        refName={refName}
        branches={branches}
        tags={tags}
        emptyRepo={emptyRepo}
        crumbs={crumbs}
        hasBlob={blob != null}
        compareOpen={compareOpen}
        onCompareOpenChange={setCompareOpen}
        onSelectRef={(ref) => setParams({ ref, path: null, file: null })}
        onOpenRefs={openRefs}
        onOpenDir={openDir}
      />

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

      <RepoCodeBody
        owner={owner}
        name={name}
        refName={refName}
        locale={locale}
        latestCommit={latestCommit}
        error={error}
        emptyRepo={emptyRepo}
        commands={commands}
        copy={copy}
        blob={blob}
        blame={blame}
        blameParam={blameParam}
        codeHostRef={codeHostRef}
        onToggleBlame={() => setParams({ blame: blameParam ? null : "1" })}
        onEditBlob={openEditDialog}
        onDeleteBlob={(p) => removeEntry(p, false)}
        onOpenRepoLink={openRepoLink}
        entries={entries}
        currentDir={currentDir}
        onOpenEntry={openEntry}
        onEditPath={openEditDialog}
        onRenamePath={renameEntry}
        onRemoveEntry={removeEntry}
        readmeContent={readmeContent}
        readmeEntryName={readmeEntryName}
        readmePath={readmePath}
      />

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

