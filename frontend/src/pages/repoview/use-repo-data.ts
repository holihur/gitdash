import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import {
  api,
  type Blame,
  type Blob,
  type Branch,
  type Commit,
  type Repo,
  type Tag,
  type TreeEntry,
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/**
 * RepoView 的数据层：仓库元信息、分支/标签、目录树、blob/blame 与目录 README。
 * 只负责取数与派生，不涉及交互动作。
 */
export function useRepoData({
  owner,
  name,
  currentDir,
  fileParam,
  blameParam,
  urlRef,
  setParams,
}: {
  owner: string;
  name: string;
  currentDir: string;
  fileParam: string;
  blameParam: boolean;
  urlRef: string;
  setParams: (patch: Record<string, string | null>) => void;
}) {
  const { to } = useI18n();
  const [repo, setRepo] = useState<Repo | null>(null);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [entries, setEntries] = useState<TreeEntry[]>([]);
  const [treeLatestCommit, setTreeLatestCommit] = useState<Commit | null>(null);
  const [blob, setBlob] = useState<Blob | null>(null);
  const [blame, setBlame] = useState<Blame | null>(null);
  const [error, setError] = useState("");
  const [missing, setMissing] = useState(false);
  const [readmeContent, setReadmeContent] = useState<string | null>(null);
  const [loadTick, setLoadTick] = useState(0);
  const [tags, setTags] = useState<Tag[]>([]);

  const ref =
    urlRef ||
    branches.find((b) => b.is_head)?.name ||
    repo?.default_branch ||
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

  const refreshBranches = useCallback(async () => {
    try {
      setBranches(await api.branches(owner, name));
    } catch {
      /* ignore */
    }
  }, [owner, name]);

  const reloadTree = useCallback(() => setLoadTick((n) => n + 1), []);

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
      setTreeLatestCommit(data.latest_commit ?? null);
      // 不要在这里清空 blob：打开子目录中的文件时 currentDir 会变为 ""，
      // 该回调会重新按根目录取树，若清空 blob 会把刚加载的文件内容冲掉。
      // blob 的清理由下方 fileParam 为空时的副作用负责。
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [name, owner, ref, currentDir, loadTick]);

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
      .catch((err) => {
        if (!alive) return;
        setBlob(null);
        // 目录链接可能没有尾随 `/`（Markdown 相对链接常见）：先按目录尝试。
        void (async () => {
          try {
            await api.tree(owner, name, ref, fileParam);
            if (!alive) return;
            setParams({ path: fileParam, file: null, line: null, blame: null });
          } catch {
            if (!alive) return;
            setParams({ file: null });
            toast.error(apiErrorMsg(to, err));
          }
        })();
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

  return {
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
    readmeEntryName: readmeEntry?.name ?? null,
    tags,
    ref,
    refreshRefs,
    refreshBranches,
    reloadTree,
  };
}
