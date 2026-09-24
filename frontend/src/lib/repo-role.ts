/** 仓库角色（从低到高）：read < triage < write < maintain < admin < owner。 */
export type RepoRole = "read" | "triage" | "write" | "maintain" | "admin" | "owner";

/** 可授予协作者的角色（不含 owner）。 */
export const COLLAB_ROLES: RepoRole[] = ["read", "triage", "write", "maintain", "admin"];

const RANKS: Record<string, number> = {
  read: 1,
  triage: 2,
  write: 3,
  maintain: 4,
  admin: 5,
  owner: 6,
};

/** 角色是否达到 min 等级。 */
export function roleAtLeast(role: string | null | undefined, min: RepoRole): boolean {
  return (RANKS[role ?? ""] ?? 0) >= RANKS[min];
}

/** 只读及以上。 */
export const canRead = (role?: string | null) => roleAtLeast(role, "read");
/** 可管理议题 / PR（标签、里程碑、指派、关闭）。 */
export const canTriage = (role?: string | null) => roleAtLeast(role, "triage");
/** 可推代码 / 文件操作 / release / 触发流水线。 */
export const canWrite = (role?: string | null) => roleAtLeast(role, "write");
/** 可评审 PR（批准 / 请求修改）。 */
export const canReview = (role?: string | null) => roleAtLeast(role, "triage");
/** 可合并 PR。 */
export const canMerge = (role?: string | null) => roleAtLeast(role, "write");
/** 可改仓库设置（webhook、deploy key、分支保护、环境变量/密钥）。 */
export const canMaintain = (role?: string | null) => roleAtLeast(role, "maintain");
/** 可管理协作者与可见性。 */
export const canAdmin = (role?: string | null) => roleAtLeast(role, "admin");
/** 仓库所有者（用户本人 / 组织 owner）。 */
export const isRepoOwner = (role?: string | null) => roleAtLeast(role, "owner");
