import { pageQuery, req, reqPage } from "./core";
import type { Blame, Blob, Branch, Commit, GlobalSearchResult, PullDiff, Repo, Tag, TreeEntry } from "./types";

export const reposApi = {
  // repos（所有仓库级操作使用 owner 限定的 URL，协作者也可访问）
  listRepos: (limit?: number, offset?: number) =>
    reqPage<Repo[]>(`/repos${pageQuery(limit, offset)}`),
  createRepo: (
    name: string,
    description: string,
    template?: "" | "readme",
    private_?: boolean,
    namespace?: string,
    templateRepo?: { owner: string; name: string },
  ) =>
    req<Repo>("/repos", {
      method: "POST",
      body: JSON.stringify({
        name,
        description,
        template: template ?? "",
        private: private_ ?? true,
        namespace: namespace || undefined,
        template_owner: templateRepo?.owner || undefined,
        template_name: templateRepo?.name || undefined,
      }),
    }),
  setRepoVisibility: (owner: string, name: string, private_: boolean) =>
    req<Repo>(`/users/${owner}/repos/${name}/visibility`, {
      method: "POST",
      body: JSON.stringify({ private: private_ }),
    }),
  setRepoTemplate: (owner: string, name: string, isTemplate: boolean) =>
    req<Repo>(`/users/${owner}/repos/${name}/template`, {
      method: "POST",
      body: JSON.stringify({ is_template: isTemplate }),
    }),
  listExplore: (limit?: number, offset?: number) =>
    reqPage<Repo[]>(`/explore/repos${pageQuery(limit, offset)}`),
  listTemplateRepos: () => req<Repo[]>("/templates"),
  globalSearch: (q: string) =>
    req<GlobalSearchResult>(`/search?q=${encodeURIComponent(q)}`),
  getRepo: (owner: string, name: string) => req<Repo>(`/users/${owner}/repos/${name}`),
  deleteRepo: (owner: string, name: string) =>
    req<null>(`/users/${owner}/repos/${name}`, { method: "DELETE" }),


  // star & fork
  listStarred: () => req<Repo[]>("/starred"),
  star: (owner: string, name: string) =>
    req<{ starred: boolean; stars: number }>(`/users/${owner}/repos/${name}/star`, {
      method: "PUT",
    }),
  unstar: (owner: string, name: string) =>
    req<{ starred: boolean; stars: number }>(`/users/${owner}/repos/${name}/star`, {
      method: "DELETE",
    }),


  // push mirror（同步到第三方）
  getMirror: (owner: string, name: string) =>
    req<{ url: string; created_at: string; status?: string; error?: string }>(
      `/users/${owner}/repos/${name}/mirror`,
    ),
  setMirror: (owner: string, name: string, url: string, privateKey?: string) =>
    req<{ url: string; created_at: string; status?: string; error?: string }>(
      `/users/${owner}/repos/${name}/mirror`,
      {
        method: "PUT",
        body: JSON.stringify({ url, private_key: privateKey ?? "" }),
      },
    ),
  deleteMirror: (owner: string, name: string) =>
    req<null>(`/users/${owner}/repos/${name}/mirror`, { method: "DELETE" }),
  syncMirror: (owner: string, name: string) =>
    req<{ status: string }>(`/users/${owner}/repos/${name}/mirror/sync`, { method: "POST" }),


  // git browsing
  branches: (owner: string, name: string) =>
    req<Branch[]>(`/users/${owner}/repos/${name}/branches`),
  tree: (owner: string, name: string, ref: string, path: string) =>
    req<{ path: string; entries: TreeEntry[] }>(
      `/users/${owner}/repos/${name}/tree?ref=${encodeURIComponent(ref)}&path=${encodeURIComponent(path)}`,
    ),
  blob: (owner: string, name: string, ref: string, path: string) =>
    req<Blob>(
      `/users/${owner}/repos/${name}/blob?ref=${encodeURIComponent(ref)}&path=${encodeURIComponent(path)}`,
    ),
  commits: (owner: string, name: string, ref: string) =>
    req<Commit[]>(`/users/${owner}/repos/${name}/commits?ref=${encodeURIComponent(ref)}`),
  blame: (owner: string, name: string, ref: string, path: string) =>
    req<Blame>(
      `/users/${owner}/repos/${name}/blame?ref=${encodeURIComponent(ref)}&path=${encodeURIComponent(path)}`,
    ),
  commitDiff: (owner: string, name: string, sha: string) =>
    req<PullDiff>(`/users/${owner}/repos/${name}/commits/${sha}/diff`),
  createCommit: (
    owner: string,
    name: string,
    branch: string,
    message: string,
    changes: { path: string; action: "create" | "update" | "delete" | "delete_tree"; content?: string }[],
  ) =>
    req<{ sha: string; branch: string; message: string }>(
      `/users/${owner}/repos/${name}/commits`,
      { method: "POST", body: JSON.stringify({ branch, message, changes }) },
    ),


  // branches & tags
  listTags: (owner: string, name: string) =>
    req<Tag[]>(`/users/${owner}/repos/${name}/tags`),
  createRef: (
    owner: string,
    name: string,
    type: "branch" | "tag",
    refName: string,
    from: string,
  ) =>
    req<{ type: string; name: string; sha: string }>(
      `/users/${owner}/repos/${name}/refs`,
      { method: "POST", body: JSON.stringify({ type, name: refName, from }) },
    ),
  deleteRef: (owner: string, name: string, type: "branch" | "tag", refName: string) =>
    req<null>(`/users/${owner}/repos/${name}/refs/${type}/${encodeURIComponent(refName)}`, {
      method: "DELETE",
    }),

};
