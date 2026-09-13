import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight, FileText, Folder, FolderOpen } from "lucide-react";
import { api, type TreeEntry } from "@/lib/api";
import { cn } from "@/lib/utils";

interface FileTreeProps {
  owner: string;
  name: string;
  refName: string;
  /** 当前目录路径（"" 表示根目录） */
  currentDir: string;
  /** 当前打开的文件路径（"" 表示未打开文件） */
  activeFile: string;
  emptyRepo: boolean;
  onOpenDir: (dir: string) => void;
  onOpenFile: (file: string) => void;
}

/**
 * 代码浏览左侧目录树：按需懒加载每个目录（复用 GET /tree），
 * 自动展开到当前目录/文件所在路径。
 */
export default function FileTree({
  owner,
  name,
  refName,
  currentDir,
  activeFile,
  emptyRepo,
  onOpenDir,
  onOpenFile,
}: FileTreeProps) {
  // 目录路径 -> 子条目（懒加载缓存），key 为 "" 表示根目录
  const [children, setChildren] = useState<Record<string, TreeEntry[]>>({});
  const [expanded, setExpanded] = useState<Set<string>>(new Set([""]));
  const [loading, setLoading] = useState<Set<string>>(new Set());
  // 用 ref 跟踪缓存，避免 loadDir 闭包陈旧
  const childrenRef = useRef(children);
  childrenRef.current = children;

  const loadDir = useCallback(
    async (dir: string) => {
      if (childrenRef.current[dir]) return; // 已加载
      setLoading((prev) => new Set(prev).add(dir));
      try {
        const data = await api.tree(owner, name, refName, dir);
        setChildren((prev) => ({ ...prev, [dir]: data.entries }));
      } catch {
        /* 目录不可访问时忽略（如权限变化/分支切换） */
      } finally {
        setLoading((prev) => {
          const next = new Set(prev);
          next.delete(dir);
          return next;
        });
      }
    },
    [owner, name, refName],
  );

  // 切换 ref 时清空缓存并重新加载根目录
  useEffect(() => {
    setChildren({});
    setExpanded(new Set([""]));
    setLoading(new Set());
    void loadDir("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [owner, name, refName]);

  // 自动展开当前目录/文件的所有祖先目录
  useEffect(() => {
    if (emptyRepo) return;
    const target = (activeFile || currentDir).split("/").filter(Boolean);
    const dirs: string[] = [""];
    // 文件路径去掉最后一段（文件名本身）；目录路径保留全部段
    const segments = activeFile ? target.slice(0, -1) : target;
    let acc = "";
    for (const seg of segments) {
      acc = acc ? `${acc}/${seg}` : seg;
      dirs.push(acc);
    }
    setExpanded((prev) => {
      const next = new Set(prev);
      for (const d of dirs) next.add(d);
      return next;
    });
    for (const d of dirs) void loadDir(d);
  }, [currentDir, activeFile, emptyRepo, loadDir]);

  const toggle = (dir: string) => {
    const isOpen = expanded.has(dir);
    setExpanded((prev) => {
      const next = new Set(prev);
      if (isOpen) next.delete(dir);
      else next.add(dir);
      return next;
    });
    if (!isOpen) void loadDir(dir);
  };

  if (emptyRepo) return null;

  const renderEntries = (entries: TreeEntry[], dir: string, depth: number) =>
    entries.map((entry) => {
      const full = dir ? `${dir}/${entry.name}` : entry.name;
      const isDir = entry.type === "tree";
      const isOpen = expanded.has(full);
      const isActive = isDir ? currentDir === full : activeFile === full;
      const pad = { paddingLeft: `${depth * 12 + 8}px` };

      if (isDir) {
        return (
          <div key={full}>
            <button
              type="button"
              className={cn(
                "flex w-full items-center gap-1.5 rounded-md py-1 pr-2 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground",
                isActive && "bg-muted font-medium text-foreground",
              )}
              style={pad}
              onClick={() => {
                onOpenDir(full);
                toggle(full);
              }}
            >
              {isOpen ? (
                <ChevronDown className="h-3.5 w-3.5 shrink-0" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5 shrink-0" />
              )}
              {isOpen ? (
                <FolderOpen className="h-4 w-4 shrink-0 text-blue-500" />
              ) : (
                <Folder className="h-4 w-4 shrink-0 text-blue-500" />
              )}
              <span className="truncate">{entry.name}</span>
            </button>
            {isOpen && children[full] && (
              <div>{renderEntries(children[full], full, depth + 1)}</div>
            )}
            {isOpen && !children[full] && loading.has(full) && (
              <div className="py-0.5 text-xs text-muted-foreground" style={pad}>
                …
              </div>
            )}
          </div>
        );
      }

      return (
        <button
          key={full}
          type="button"
          className={cn(
            "flex w-full items-center gap-1.5 rounded-md py-1 pr-2 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground",
            isActive && "bg-muted font-medium text-foreground",
          )}
          style={pad}
          onClick={() => onOpenFile(full)}
        >
          <span className="w-3.5 shrink-0" />
          <FileText className="h-4 w-4 shrink-0" />
          <span className="truncate">{entry.name}</span>
        </button>
      );
    });

  return (
    <nav className="max-h-full overflow-auto py-1" aria-label="Files">
      {children[""] ? (
        renderEntries(children[""], "", 0)
      ) : (
        <p className="px-2 py-1 text-xs text-muted-foreground">…</p>
      )}
    </nav>
  );
}
