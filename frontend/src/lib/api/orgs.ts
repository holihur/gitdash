import { req } from "./core";
import type { Collab, Org, OrgMember, Repo } from "./types";

export const orgsApi = {
  // orgs（组织）
  listOrgs: () => req<Org[]>("/orgs"),
  createOrg: (name: string, display: string) =>
    req<Org>("/orgs", { method: "POST", body: JSON.stringify({ name, display }) }),
  deleteOrg: (name: string) => req<null>(`/orgs/${name}`, { method: "DELETE" }),
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
