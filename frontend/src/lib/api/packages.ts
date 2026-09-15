import { req } from "./core";
import type { DockerImage, PackageAuditEntry, PackageEntry } from "./types";

export const packagesApi = {
  // packages（私有包注册表）
  listPackages: (owner: string, type?: string) =>
    req<PackageEntry[]>(`/packages/${encodeURIComponent(owner)}${type ? `/${type}` : ""}`),
  // docker / OCI 私有注册表（镜像名 + tags）
  listDockerImages: (owner: string) => req<DockerImage[]>(`/packages/${encodeURIComponent(owner)}/docker`),
  deletePackage: (type: string, owner: string, name: string) =>
    req<null>(
      `/packages/${encodeURIComponent(type)}/${encodeURIComponent(owner)}/${name
        .split("/")
        .map(encodeURIComponent)
        .join("/")}`,
      { method: "DELETE" },
    ),
  listPackageAudit: (owner: string) => req<PackageAuditEntry[]>(`/packages/${encodeURIComponent(owner)}/audit`),
};
