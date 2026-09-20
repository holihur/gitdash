/**
 * 仓库页面的路径化路由。
 *
 * 采用 GitHub / Gitea 风格的路径，把「浏览位置」放进 pathname，而不是全部塞进
 * 查询参数。这样 Markdown 里的相对链接（如 `docs/x.md`、`../deps/agent`）才能被
 * 浏览器按目录结构正确解析；`ref` 仍留在查询参数里，避免分支名包含 `/` 时
 * 与文件路径产生歧义。
 *
 *   /repo/:owner/:name                       代码根目录
 *   /repo/:owner/:name/tree/<dir>            目录
 *   /repo/:owner/:name/blob/<file>           文件
 *   /repo/:owner/:name/blame/<file>          blame
 *   /repo/:owner/:name/commits|issues|...    其余 tab
 */

export const REPO_TABS = [
  "code",
  "commits",
  "issues",
  "pulls",
  "pipeline",
  "copilot",
  "releases",
  "projects",
  "settings",
] as const;

export type RepoTab = (typeof REPO_TABS)[number];
export type RepoCodeKind = "tree" | "blob" | "blame";
export type FileOpMode = "new" | "edit" | "rename";

export interface RepoRoute {
  tab: RepoTab;
  /** code tab 的浏览类型；非 code tab 恒为 tree */
  kind: RepoCodeKind;
  /** 相对仓库根的路径（无前导 / 尾随斜杠）；非 code tab 为空串 */
  path: string;
  /** issues tab 下指向的 issue 编号；0 表示列表页 */
  issueNumber: number;
}

const encodePath = (p: string): string =>
  p
    .split("/")
    .filter(Boolean)
    .map(encodeURIComponent)
    .join("/");

// 解析 issue 详情路径里的编号：仅接受正整数，其余归为列表页。
const parseIssueNumber = (s: string): number => {
  const n = Number(s);
  return Number.isInteger(n) && n > 0 ? n : 0;
};

/** 解析 `/repo/:owner/:name` 之后的 splat 部分（React Router 已解码）。 */
export function parseRepoRoute(splat: string | undefined): RepoRoute {
  const rest = (splat ?? "").replace(/^\/+/, "").replace(/\/+$/, "");
  if (!rest) return { tab: "code", kind: "tree", path: "", issueNumber: 0 };
  const slash = rest.indexOf("/");
  const head = slash < 0 ? rest : rest.slice(0, slash);
  const tail = slash < 0 ? "" : rest.slice(slash + 1);
  if (head === "tree") return { tab: "code", kind: "tree", path: tail, issueNumber: 0 };
  if (head === "blob") return { tab: "code", kind: "blob", path: tail, issueNumber: 0 };
  if (head === "blame") return { tab: "code", kind: "blame", path: tail, issueNumber: 0 };
  if (head !== "code" && (REPO_TABS as readonly string[]).includes(head)) {
    return {
      tab: head as RepoTab,
      kind: "tree",
      path: "",
      issueNumber: head === "issues" ? parseIssueNumber(tail) : 0,
    };
  }
  // 未知路径：回退代码根目录
  return { tab: "code", kind: "tree", path: "", issueNumber: 0 };
}

/** 构造 issue 详情页路径：/repo/:owner/:name/issues/<number> */
export function buildIssuePath(owner: string, name: string, number: number): string {
  return `${buildRepoPath(owner, name, { tab: "issues" })}/${number}`;
}

/**
 * 构造「新建 / 编辑 / 重命名文件」整页的 pathname（不含查询参数）。
 * 参数说明：
 *   /repo/:owner/:name/new?kind=file|dir&dir=<dir>&ref=<branch>
 *   /repo/:owner/:name/edit?path=<path>&ref=<branch>
 *   /repo/:owner/:name/rename?path=<path>&kind=file|dir&ref=<branch>
 */
export function buildFileOpPath(owner: string, name: string, mode: FileOpMode): string {
  return `/repo/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/${mode}`;
}

/** 构造仓库页面的 pathname（不含查询参数）。 */
export function buildRepoPath(
  owner: string,
  name: string,
  route: { tab: RepoTab; kind?: RepoCodeKind; path?: string },
): string {
  const base = `/repo/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`;
  if (route.tab !== "code") return `${base}/${route.tab}`;
  const p = (route.path ?? "").replace(/^\/+/, "").replace(/\/+$/, "");
  if (!p) return base;
  const kind = route.kind === "blob" || route.kind === "blame" ? route.kind : "tree";
  return `${base}/${kind}/${encodePath(p)}`;
}
