import { req } from "./core";
import type { PackageAuditEntry, PackageEntry } from "./types";

export const packagesApi = {
  // packages（私有包注册表）
  listPackages: (owner: string, type?: string) =>
    req<PackageEntry[]>(`/packages/${encodeURIComponent(owner)}${type ? `/${type}` : ""}`),
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
