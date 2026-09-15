/**
 * Markdown 仓库内引用的解析与重写。
 *
 * 背景：仓库里的 README / 文档用相对路径互相引用（如 `docs/projects.md`、
 * `../deps/agent`）或文档内锚点（如 `#backup--restore`）。这些链接若原样渲染，
 * 浏览器会按当前 URL（`/repo/:owner/:name`）解析，最终落到不存在的路由上。
 * 这里在渲染阶段把它们改写成路径化的代码浏览地址（`/blob/...`、`/tree/...`），
 * 并统一标题锚点 id，使仓库内引用能够正确跳转。
 */

import { buildRepoPath } from "@/lib/repo-url";

/** 当前 Markdown 文件所在的仓库上下文（用于解析相对引用）。 */
export interface RepoLinkContext {
  owner: string;
  name: string;
  /** 当前浏览的 ref（分支 / 标签 / commit），跳转时保持同一引用。 */
  ref: string;
  /** 当前 Markdown 文件在仓库中的路径（相对根，如 `README.md` / `docs/guide.md`）。 */
  path: string;
}

/** 解析后的仓库内链接目标。 */
export interface RepoLinkTarget {
  /** 仓库内绝对路径（相对根，无前导斜杠）。 */
  path: string;
  kind: "file" | "dir";
  /** 原链接中的锚点（不含 `#`），跨文件定位标题时使用。 */
  hash?: string;
}

// GitHub 风格 slug：去 HTML、去标点、空白转连字符；不做连续连字符合并，
// 因此 "Backup & Restore" -> "backup--restore"（与 GitHub 生成的锚点一致）。
const PUNCT_RE = /[\u2000-\u206F\u2E00-\u2E7F\\'!"#$%&()*+,./:;<=>?@[\]^`{|}~]/g;

export function slugifyHeading(text: string): string {
  return (text ?? "")
    .toLowerCase()
    .trim()
    .replace(/<[!/a-z].*?>/gi, "")
    .replace(PUNCT_RE, "")
    .replace(/\s/g, "-");
}

/** 判断链接是否指向站外 / 非仓库资源（协议、协议相对、页内查询）。 */
export function isExternalHref(href: string): boolean {
  return /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(href) || href.startsWith("//") || href.startsWith("?");
}

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

/**
 * 把 `href` 解析为仓库内绝对路径（相对根，无前导斜杠）。
 * 以 `/` 开头视为仓库根；否则相对当前文件所在目录解析；`..` 不会越过仓库根。
 * 片段（`#`）与查询（`?`）会被剥离。
 */
export function resolveRepoPath(fromPath: string, href: string): string {
  const clean = href.split("#")[0].split("?")[0];
  const decoded = safeDecode(clean);
  const baseDir = fromPath.includes("/") ? fromPath.slice(0, fromPath.lastIndexOf("/")) : "";
  const raw = decoded.startsWith("/")
    ? decoded.replace(/^\/+/, "")
    : baseDir
      ? `${baseDir}/${decoded}`
      : decoded;
  const out: string[] = [];
  for (const seg of raw.split("/")) {
    if (seg === "" || seg === ".") continue;
    if (seg === "..") {
      out.pop();
      continue;
    }
    out.push(seg);
  }
  return out.join("/");
}

/** 把 Markdown 链接解析成仓库内目标；站外链接、页内锚点、非仓库链接返回 null。 */
export function parseRepoLink(ctx: RepoLinkContext, href: string): RepoLinkTarget | null {
  const trimmed = (href ?? "").trim();
  if (!trimmed || trimmed.startsWith("#") || isExternalHref(trimmed)) return null;
  // 已经是本应用生成的仓库 URL，避免重复改写
  if (trimmed.startsWith("/repo/")) return null;

  const hashIdx = trimmed.indexOf("#");
  const beforeHash = hashIdx >= 0 ? trimmed.slice(0, hashIdx) : trimmed;
  const hash = hashIdx >= 0 ? trimmed.slice(hashIdx + 1) : "";
  const kind: RepoLinkTarget["kind"] = beforeHash.endsWith("/") ? "dir" : "file";
  // 相对路径越出仓库根时按 GitHub 行为裁剪到根；路径为空表示仓库根目录
  const path = resolveRepoPath(ctx.path, beforeHash);
  return { path, kind, hash: hash || undefined };
}

/** 构造仓库内目标的路径化地址（保持当前 ref）。 */
export function buildRepoHref(ctx: RepoLinkContext, target: RepoLinkTarget): string {
  const pathname = buildRepoPath(ctx.owner, ctx.name, {
    tab: "code",
    kind: target.kind === "dir" ? "tree" : "blob",
    path: target.path,
  });
  const params = new URLSearchParams();
  if (ctx.ref) params.set("ref", ctx.ref);
  const qs = params.toString();
  const hash = target.hash ? `#${target.hash}` : "";
  return `${pathname}${qs ? `?${qs}` : ""}${hash}`;
}

/**
 * 从已渲染的 DOM 收集标题 slug -> 锚点 id 的映射，重名标题按 GitHub 规则追加 `-1`、`-2`。
 * 供页内锚点与跨文件锚点定位使用。
 */
export function headingAnchorMap(root: ParentNode): Map<string, string> {
  const map = new Map<string, string>();
  const seen = new Map<string, number>();
  root.querySelectorAll<HTMLElement>("h1, h2, h3").forEach((el) => {
    if (!el.id) return;
    let slug = slugifyHeading(el.textContent ?? "");
    if (!slug) return;
    const n = seen.get(slug) ?? 0;
    seen.set(slug, n + 1);
    if (n > 0) slug = `${slug}-${n}`;
    if (!map.has(slug)) map.set(slug, el.id);
  });
  return map;
}

/**
 * 重写 Markdown 渲染后的 HTML：
 * - 为 h1-h3 分配与目录一致的锚点 id（`md-heading-N`）；
 * - 页内锚点 `#slug` 指向对应标题 id；
 * - 仓库内相对引用改写为代码浏览路由，并写入 `data-repo-*` 供点击拦截。
 */
export function rewriteMarkdownHtml(html: string, repo?: RepoLinkContext): string {
  if (typeof DOMParser === "undefined") return html;
  const doc = new DOMParser().parseFromString(html, "text/html");
  doc.querySelectorAll<HTMLElement>("h1, h2, h3").forEach((el, i) => {
    if (!el.id) el.id = `md-heading-${i}`;
  });
  const anchors = headingAnchorMap(doc);

  doc.querySelectorAll<HTMLAnchorElement>("a[href]").forEach((a) => {
    const href = a.getAttribute("href") ?? "";
    if (href.startsWith("#")) {
      const id = anchors.get(safeDecode(href.slice(1)));
      if (id) a.setAttribute("href", `#${id}`);
      return;
    }
    if (!repo) return;
    const target = parseRepoLink(repo, href);
    if (!target) return;
    a.setAttribute("href", buildRepoHref(repo, target));
    a.setAttribute("data-repo-path", target.path);
    a.setAttribute("data-repo-kind", target.kind);
    if (target.hash) a.setAttribute("data-repo-hash", target.hash);
  });

  return doc.body.innerHTML;
}
