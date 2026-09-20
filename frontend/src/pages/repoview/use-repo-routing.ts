import { useCallback, useEffect, useMemo } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { buildRepoPath, parseRepoRoute, type RepoCodeKind, type RepoTab } from "@/lib/repo-url";

export interface RepoRouting {
  owner: string;
  name: string;
  tab: RepoTab;
  fileParam: string;
  path: string[];
  currentDir: string;
  blameParam: boolean;
  urlRef: string;
  lineParam: number | null;
  /** issues tab 下的 issue 编号（0 = 列表页） */
  issueNumber: number;
  setParams: (patch: Record<string, string | null>) => void;
}

/**
 * 把 /repo/:owner/:name/* 的路径化路由解析为可供页面使用的派生值，
 * 并返回一个兼容旧查询参数语义的 setParams。
 */
export function useRepoRouting(): RepoRouting {
  const { owner = "", name = "", "*": splat = "" } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

  // 路径化路由派生：/repo/:owner/:name/{tree|blob|blame}/... 及其余 tab。
  // ref / line 仍走查询参数（ref 可能包含 `/`，放路径里会与文件路径歧义）。
  const route = useMemo(() => parseRepoRoute(splat), [splat]);
  const tab: RepoTab = route.tab;
  const fileParam = route.tab === "code" && route.kind !== "tree" ? route.path : "";
  const path =
    route.tab === "code" && route.kind === "tree" && route.path ? route.path.split("/") : [];
  const currentDir = route.tab === "code" && route.kind === "tree" ? route.path : "";
  const blameParam = route.tab === "code" && route.kind === "blame";
  const urlRef = searchParams.get("ref") ?? "";
  const lineParam = Number(searchParams.get("line")) || null;
  const issueNumber = route.issueNumber;

  // 兼容既有 `setParams` 语义（CodeTab / tab 切换均调用），把变更翻译为路径化导航。
  const setParams = useCallback(
    (patch: Record<string, string | null>) => {
      const nextTab: RepoTab = "tab" in patch ? ((patch.tab as RepoTab) ?? "code") : tab;
      let kind: RepoCodeKind = route.kind;
      let dirPath = route.kind === "tree" ? route.path : "";
      let filePath = route.kind !== "tree" ? route.path : "";
      let nextBlame = route.kind === "blame";
      let nextRef = urlRef;
      let nextLine: number | null = lineParam;
      let nextHash = "";

      if ("path" in patch) dirPath = patch.path ?? "";
      if ("file" in patch) {
        filePath = patch.file ?? "";
        kind = filePath ? (nextBlame ? "blame" : "blob") : "tree";
      }
      if ("blame" in patch) {
        nextBlame = patch.blame === "1" || patch.blame === "true";
        if (filePath) kind = nextBlame ? "blame" : "blob";
      }
      if ("ref" in patch) nextRef = patch.ref ?? "";
      if ("line" in patch) nextLine = patch.line ? Number(patch.line) : null;
      // 支持跨文件锚点：patch.hash 不进查询参数，只拼成 URL 片段。
      if ("hash" in patch) nextHash = patch.hash ?? "";

      const pathname =
        nextTab === "code"
          ? buildRepoPath(owner, name, {
              tab: "code",
              kind,
              path: kind === "tree" ? dirPath : filePath,
            })
          : buildRepoPath(owner, name, { tab: nextTab });

      const next = new URLSearchParams(searchParams);
      if (nextRef) next.set("ref", nextRef);
      else next.delete("ref");
      if (nextTab === "code" && nextLine && filePath) next.set("line", String(nextLine));
      else next.delete("line");
      const qs = next.toString();
      navigate(`${pathname}${qs ? `?${qs}` : ""}${nextHash ? `#${nextHash}` : ""}`);
    },
    [owner, name, tab, route, urlRef, lineParam, searchParams, navigate],
  );

  // 兼容旧的查询参数式 URL（?tab=&path=&file=&blame=），自动重定向到路径化地址。
  useEffect(() => {
    const legacyTab = searchParams.get("tab");
    const legacyPath = searchParams.get("path") ?? "";
    const legacyFile = searchParams.get("file") ?? "";
    const legacyBlame = searchParams.get("blame") === "1";
    if (!legacyTab && !legacyPath && !legacyFile && !legacyBlame) return;
    const kind: RepoCodeKind = legacyFile ? (legacyBlame ? "blame" : "blob") : "tree";
    const pathname =
      legacyTab && legacyTab !== "code"
        ? buildRepoPath(owner, name, { tab: legacyTab as RepoTab })
        : buildRepoPath(owner, name, { tab: "code", kind, path: legacyFile || legacyPath });
    const next = new URLSearchParams(searchParams);
    next.delete("tab");
    next.delete("path");
    next.delete("file");
    next.delete("blame");
    const qs = next.toString();
    navigate(`${pathname}${qs ? `?${qs}` : ""}`, { replace: true });
  }, [searchParams, navigate, owner, name]);

  return { owner, name, tab, fileParam, path, currentDir, blameParam, urlRef, lineParam, issueNumber, setParams };
}
