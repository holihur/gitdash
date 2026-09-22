import { req, sendForm } from "./core";
import type { Collab, Org, OrgFollowState, OrgMember, OrgProfile, Repo, UserSummary } from "./types";

export const orgsApi = {
  // orgs（组织）
  listOrgs: () => req<Org[]>("/orgs"),
  createOrg: (name: string, display: string) =>
    req<Org>("/orgs", { method: "POST", body: JSON.stringify({ name, display }) }),
  /** 修改组织信息（仅 owner）：display / bio。 */
  updateOrg: (org: string, patch: { display?: string; bio?: string }) =>
    req<Org>(`/orgs/${encodeURIComponent(org)}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  deleteOrg: (name: string) => req<null>(`/orgs/${name}`, { method: "DELETE" }),
  getOrgProfile: (org: string) => req<OrgProfile>(`/orgs/${org}/profile`),
  /** 上传组织封面（仅 owner）。 */
  uploadOrgCover: (org: string, file: File) => {
    const form = new FormData();
    form.append("cover", file);
    return sendForm<{ cover_url: string }>(`/orgs/${org}/cover`, form);
  },
  /** 删除组织封面（仅 owner）。 */
  deleteOrgCover: (org: string) =>
    req<{ deleted: boolean }>(`/orgs/${org}/cover`, { method: "DELETE" }),
  followOrg: (org: string) => req<OrgFollowState>(`/orgs/${org}/follow`, { method: "POST" }),
  unfollowOrg: (org: string) => req<OrgFollowState>(`/orgs/${org}/follow`, { method: "DELETE" }),
  listOrgFollowers: (org: string) => req<UserSummary[]>(`/orgs/${org}/followers`),
  listOrgMembers: (org: string) => req<OrgMember[]>(`/orgs/${org}/members`),
  addOrgMember: (org: string, username: string, role: string) =>
    req<{ org: string; username: string; role: string }>(`/orgs/${org}/members`, {
      method: "POST",
      body: JSON.stringify({ username, role }),
    }),
  removeOrgMember: (org: string, username: string) =>
    req<null>(`/orgs/${org}/members/${username}`, { method: "DELETE" }),
  listOrgRepos: (org: string) => req<{ role: string; repos: Repo[] }>(`/orgs/${org}/repos`),


  // collaborators
  listCollabs: (owner: string, name: string) =>
    req<Collab[]>(`/users/${owner}/repos/${name}/collabs`),
  addCollab: (owner: string, name: string, username: string, permission: "read" | "write") =>
    req<Collab>(`/users/${owner}/repos/${name}/collabs`, {
      method: "POST",
      body: JSON.stringify({ username, permission }),
    }),
  removeCollab: (owner: string, name: string, username: string) =>
    req<null>(`/users/${owner}/repos/${name}/collabs/${username}`, { method: "DELETE" }),

};
