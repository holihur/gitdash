import { req, reqPage, send } from "./core";
import type { DockerImage, PackageAuditEntry, PackageEntry, PackageFileEntry } from "./types";

/** 包名可能含 `/`（composer vendor/name、maven group/artifact、go module 路径）。 */
const encPath = (name: string) =>
  name
    .split("/")
    .map(encodeURIComponent)
    .join("/");

/** 构造网页端包文件接口的 URL（相对 /api）。 */
export function packageFilePath(
  type: string,
  owner: string,
  name: string,
  query?: Record<string, string | undefined>,
): string {
  const base = `/package-files/${encodeURIComponent(type)}/${encodeURIComponent(owner)}/${encPath(name)}`;
  if (!query) return base;
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v != null && v !== "") qs.set(k, v);
  }
  const s = qs.toString();
  return s ? `${base}?${s}` : base;
}

/** 包详情页路径（/packages/:type/:owner/:name...）。 */
export function packageDetailPath(type: string, owner: string, name: string): string {
  return `/packages/${encodeURIComponent(type)}/${encodeURIComponent(owner)}/${encPath(name)}`;
}

export const packagesApi = {
  // packages（私有包注册表）
  listPackages: (owner: string, type?: string) =>
    req<PackageEntry[]>(`/packages/${encodeURIComponent(owner)}${type ? `/${type}` : ""}`),
  /** 带搜索 + 分页的包列表（返回 X-Total-Count 总数）。 */
  listPackagesPage: (
    owner: string,
    type: string | undefined,
    opts: { q?: string; limit: number; offset: number },
  ) => {
    const p = new URLSearchParams({ limit: String(opts.limit), offset: String(opts.offset) });
    if (opts.q) p.set("q", opts.q);
    return reqPage<PackageEntry[]>(
      `/packages/${encodeURIComponent(owner)}${type ? `/${type}` : ""}?${p.toString()}`,
    );
  },
  // docker / OCI 私有注册表（镜像名 + tags）
  listDockerImages: (owner: string) => req<DockerImage[]>(`/packages/${encodeURIComponent(owner)}/docker`),
  deletePackage: (type: string, owner: string, name: string) =>
    req<null>(
      `/packages/${encodeURIComponent(type)}/${encodeURIComponent(owner)}/${encPath(name)}`,
      { method: "DELETE" },
    ),
  listPackageAudit: (owner: string) => req<PackageAuditEntry[]>(`/packages/${encodeURIComponent(owner)}/audit`),

  // 包文件浏览（网页端）
  listPackageFiles: (type: string, owner: string, name: string) =>
    req<PackageEntry[]>(packageFilePath(type, owner, name)),
  listPackageEntries: (type: string, owner: string, name: string, version: string, filename: string) =>
    req<PackageFileEntry[]>(
      packageFilePath(type, owner, name, { version, filename, entries: "1" }),
    ),
  /** 读取归档内文本条目（用于预览）。 */
  readPackageEntry: (type: string, owner: string, name: string, version: string, filename: string, entry: string) =>
    send(packageFilePath(type, owner, name, { version, filename, entry })).then((r) => r.text()),
};
