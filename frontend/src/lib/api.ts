// 会话通过 httpOnly Cookie(gitdash_session) 自动携带，前端不再持有 token。

/** 带后端错误码的 API 错误（code 供前端 i18n 映射，缺失时用 message 兜底） */
export class ApiError extends Error {
  status: number;
  code?: string;

  constructor(status: number, message: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

export function getToken(): string {
  return "";
}

export function setToken(_t: string) {
  /* cookie-based sessions */
}

export function clearToken() {
  /* cookie-based sessions */
}

export interface User {
  username: string;
  email?: string;
  created_at: string;
  mfa_enabled: boolean;
}

export interface LoginResult {
  token?: string;
  username?: string;
  mfa_required?: boolean;
  mfa_token?: string;
}

export interface MFAStatus {
  enabled: boolean;
  pending_secret?: string;
  otpauth_url?: string;
}

export interface MFAEnroll {
  secret: string;
  otpauth_url: string;
}

export interface Repo {
  id: number;
  owner: string;
  name: string;
  description: string;
  created_at: string;
  private?: boolean;
  /** 仅“可访问仓库列表”返回：owner / read / write */
  role?: "owner" | "read" | "write";
  /** star 数量与当前用户是否已 star */
  stars?: number;
  starred?: boolean;
  /** watch 数量与当前用户是否已 watch */
  watchers?: number;
  watching?: boolean;
  /** fork 来源（仅 fork 仓库） */
  fork_owner?: string;
  fork_repo?: string;
}

export interface GPGKey {
  id: number;
  fingerprint: string;
  created_at: string;
}

export interface Org {
  name: string;
  display: string;
  created_at: string;
  role: string;
}

export interface OrgMember {
  org: string;
  username: string;
  role: string;
}

export interface SSHKey {
  id: number;
  name: string;
  public_key: string;
  fingerprint: string;
  created_at: string;
}

export interface PAT {
  id: number;
  name: string;
  scopes: string[];
  created_at: string;
  last_used_at: string;
}

export interface CreatedPAT extends PAT {
  token: string;
}

export interface Branch {
  name: string;
  is_head: boolean;
}

export interface TreeEntry {
  name: string;
  type: "blob" | "tree";
  mode: string;
  size: number;
  sha: string;
  modified_at?: string;
  modified_by?: string;
  modified_msg?: string;
  last_commit?: string;
}

export interface Blob {
  path: string;
  size: number;
  encoding: "utf-8" | "binary" | "truncated";
  content: string;
}

export interface BlameCommit {
  sha: string;
  author: string;
  date: string;
  message: string;
}

export interface BlameLine {
  line: number;
  commit: string;
  content: string;
}

export interface Blame {
  path: string;
  commits: Record<string, BlameCommit>;
  lines: BlameLine[];
}

export interface Commit {
  sha: string;
  author: string;
  date: string;
  message: string;
  /** 经已注册 GPG 公钥验证的提交所属用户 */
  gpg_verified?: string;
}
export interface Issue {
  id: number;
  number: number;
  title: string;
  body: string;
  state: "open" | "closed";
  author: string;
  created_at: string;
  updated_at: string;
  closed_at: string | null;
  /** 服务端 enrich：所属标签与里程碑 */
  labels?: Label[];
  milestone?: Milestone | null;
}

export interface Label {
  id: number;
  name: string;
  color: string;
}

export interface Milestone {
  id: number;
  title: string;
  description: string;
  state: "open" | "closed";
  open_issues: number;
  closed_issues: number;
}

export interface IssueComment {
  id: number;
  number: number;
  author: string;
  body: string;
  created_at: string;
  updated_at: string;
}

export interface Collab {
  owner: string;
  repo: string;
  username: string;
  permission: "read" | "write";
  created_at: string;
}

export type PullState = "open" | "merged" | "closed";

export interface PullRequest {
  id: number;
  number: number;
  title: string;
  body: string;
  source_branch: string;
  target_branch: string;
  base_sha: string;
  head_sha: string;
  state: PullState;
  author: string;
  created_at: string;
  updated_at: string;
  merged_at: string | null;
  merged_by: string;
}

export interface Tag {
  name: string;
  sha: string;
  message: string;
}

export interface PullDiff {
  files: { path: string; status: "A" | "M" | "D"; insertions: number; deletions: number }[];
  patch: string;
  base_sha: string;
  head_sha: string;
}

export interface Webhook {
  id: number;
  owner: string;
  repo: string;
  url: string;
  created_at: string;
}

export type PipelineRunStatus = "pending" | "running" | "success" | "failed";

export interface PipelineRun {
  id: number;
  sha: string;
  ref: string;
  trigger_by: string;
  status: PipelineRunStatus;
  steps_total: number;
  steps_done: number;
  error?: string;
  created_at: string;
  finished_at: string | null;
  /** 仅详情返回 */
  log?: string;
}

export type NotifKind = "issue" | "pull";
export type NotifAction = "opened" | "closed" | "reopened" | "merged";

export interface Notification {
  id: number;
  kind: NotifKind;
  action: NotifAction;
  owner: string;
  repo: string;
  number: number;
  title: string;
  actor: string;
  read: boolean;
  created_at: string;
}
export function cloneUrl(owner: string, name: string): string {
  return `ssh://git@${window.location.hostname}:2222/${owner}/${name}.git`;
}

export function cloneCommand(owner: string, name: string): string {
  return `git clone ${cloneUrl(owner, name)}`;
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...opts,
    credentials: "same-origin", // 携带 httpOnly cookie
    headers: {
      "Content-Type": "application/json",
      ...(opts.headers ?? {}),
    },
  });
  if (!res.ok) {
    if (res.status === 401 && window.location.pathname !== "/login") {
      // 会话失效：回登录页并带上回跳地址（cookie 由服务端在 logout 时清除）
      const redirect = encodeURIComponent(
        window.location.pathname + window.location.search + window.location.hash,
      );
      window.location.href = `/login?redirect=${redirect}`;
    }
    let msg = res.statusText;
    let code: string | undefined;
    try {
      const body = await res.json();
      if (typeof body?.error === "string") msg = body.error;
      if (typeof body?.code === "string") code = body.code;
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, msg, code);
  }
  if (res.status === 204) return null as T;
  return res.json();
}

export const api = {
  // auth
  register: (username: string, password: string) =>
    req<{ token: string; username: string }>("/auth/register", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  login: (username: string, password: string) =>
    req<LoginResult>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  mfaVerify: (mfaToken: string, code: string) =>
    req<{ token: string; username: string }>("/auth/mfa-verify", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, code }),
    }),
  logout: () => req<null>("/auth/logout", { method: "POST" }),
  me: () => req<User>("/me"),

  // profile
  changePassword: (currentPassword: string, newPassword: string) =>
    req<null>("/me/password", {
      method: "POST",
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),
  updateProfile: (email: string) =>
    req<{ username: string; email: string }>("/me/profile", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  mfaStatus: () => req<MFAStatus>("/me/mfa"),
  mfaEnroll: () => req<MFAEnroll>("/me/mfa/enroll", { method: "POST" }),
  mfaActivate: (code: string) =>
    req<null>("/me/mfa/activate", { method: "POST", body: JSON.stringify({ code }) }),
  mfaDisable: (password: string, code: string) =>
    req<null>("/me/mfa/disable", {
      method: "POST",
      body: JSON.stringify({ password, code }),
    }),

  // repos（所有仓库级操作使用 owner 限定的 URL，协作者也可访问）
  listRepos: () => req<Repo[]>("/repos"),
  createRepo: (
    name: string,
    description: string,
    template?: "" | "readme",
    private_?: boolean,
    namespace?: string,
  ) =>
    req<Repo>("/repos", {
      method: "POST",
      body: JSON.stringify({
        name,
        description,
        template: template ?? "",
        private: private_ ?? true,
        namespace: namespace || undefined,
      }),
    }),
  setRepoVisibility: (owner: string, name: string, private_: boolean) =>
    req<Repo>(`/users/${owner}/repos/${name}/visibility`, {
      method: "POST",
      body: JSON.stringify({ private: private_ }),
    }),
  listExplore: () => req<Repo[]>("/explore/repos"),
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

  // watch & inbox
  listWatched: () => req<Repo[]>("/watched"),
  watch: (owner: string, name: string) =>
    req<{ watching: boolean; watchers: number }>(`/users/${owner}/repos/${name}/watch`, {
      method: "PUT",
    }),
  unwatch: (owner: string, name: string) =>
    req<{ watching: boolean; watchers: number }>(`/users/${owner}/repos/${name}/watch`, {
      method: "DELETE",
    }),
  inbox: () => req<Notification[]>("/inbox"),
  inboxUnread: () => req<{ count: number }>("/inbox/unread"),
  inboxRead: (id: number) =>
    req<{ ok: boolean }>(`/inbox/read/${id}`, { method: "POST" }),
  inboxReadAll: () => req<{ ok: boolean }>("/inbox/read", { method: "POST" }),
  inboxDelete: (id: number) => req<null>(`/inbox/${id}`, { method: "DELETE" }),
  forkRepo: (owner: string, name: string, opts?: { name?: string; namespace?: string }) =>
    req<Repo>(`/users/${owner}/repos/${name}/fork`, {
      method: "POST",
      body: JSON.stringify(opts ?? {}),
    }),
  importRepo: (opts: {
    url: string;
    name?: string;
    namespace?: string;
    private?: boolean;
    private_key?: string;
  }) =>
    req<Repo>("/imports", {
      method: "POST",
      body: JSON.stringify(opts),
    }),

  // push mirror（同步到第三方）
  getMirror: (owner: string, name: string) =>
    req<{ url: string; created_at: string }>(`/users/${owner}/repos/${name}/mirror`),
  setMirror: (owner: string, name: string, url: string, privateKey?: string) =>
    req<{ url: string; created_at: string }>(`/users/${owner}/repos/${name}/mirror`, {
      method: "PUT",
      body: JSON.stringify({ url, private_key: privateKey ?? "" }),
    }),
  deleteMirror: (owner: string, name: string) =>
    req<null>(`/users/${owner}/repos/${name}/mirror`, { method: "DELETE" }),
  syncMirror: (owner: string, name: string) =>
    req<{ ok: boolean }>(`/users/${owner}/repos/${name}/mirror/sync`, { method: "POST" }),

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

  // issues
  listIssues: (owner: string, name: string) => req<Issue[]>(`/users/${owner}/repos/${name}/issues`),
  createIssue: (owner: string, name: string, title: string, body: string) =>
    req<Issue>(`/users/${owner}/repos/${name}/issues`, {
      method: "POST",
      body: JSON.stringify({ title, body }),
    }),
  setIssueState: (owner: string, name: string, number: number, state: "open" | "closed") =>
    req<Issue>(`/users/${owner}/repos/${name}/issues/${number}`, {
      method: "PATCH",
      body: JSON.stringify({ state }),
    }),

  // issue/PR comments
  listComments: (
    owner: string,
    name: string,
    number: number,
    kind: "issues" | "pulls" = "issues",
  ) => req<IssueComment[]>(`/users/${owner}/repos/${name}/${kind}/${number}/comments`),
  postComment: (
    owner: string,
    name: string,
    number: number,
    body: string,
    kind: "issues" | "pulls" = "issues",
  ) =>
    req<IssueComment>(`/users/${owner}/repos/${name}/${kind}/${number}/comments`, {
      method: "POST",
      body: JSON.stringify({ body }),
    }),
  deleteComment: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/comments/${id}`, { method: "DELETE" }),

  // issue labels
  listLabels: (owner: string, name: string) => req<Label[]>(`/users/${owner}/repos/${name}/labels`),
  createLabel: (owner: string, name: string, labelName: string, color: string) =>
    req<Label>(`/users/${owner}/repos/${name}/labels`, {
      method: "POST",
      body: JSON.stringify({ name: labelName, color }),
    }),
  updateLabel: (owner: string, name: string, id: number, labelName: string, color: string) =>
    req<Label>(`/users/${owner}/repos/${name}/labels/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name: labelName, color }),
    }),
  deleteLabel: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/labels/${id}`, { method: "DELETE" }),
  setIssueLabels: (owner: string, name: string, number: number, labelIds: number[]) =>
    req<Issue>(`/users/${owner}/repos/${name}/issues/${number}/labels`, {
      method: "POST",
      body: JSON.stringify({ label_ids: labelIds }),
    }),

  // milestones
  listMilestones: (owner: string, name: string) =>
    req<Milestone[]>(`/users/${owner}/repos/${name}/milestones`),
  createMilestone: (owner: string, name: string, title: string, description: string) =>
    req<Milestone>(`/users/${owner}/repos/${name}/milestones`, {
      method: "POST",
      body: JSON.stringify({ title, description }),
    }),
  updateMilestone: (
    owner: string,
    name: string,
    id: number,
    fields: { title?: string; description?: string; state?: "open" | "closed" },
  ) =>
    req<Milestone>(`/users/${owner}/repos/${name}/milestones/${id}`, {
      method: "PATCH",
      body: JSON.stringify(fields),
    }),
  deleteMilestone: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/milestones/${id}`, { method: "DELETE" }),
  setIssueMilestone: (owner: string, name: string, number: number, milestoneId: number) =>
    req<Issue>(`/users/${owner}/repos/${name}/issues/${number}/milestone`, {
      method: "POST",
      body: JSON.stringify({ milestone_id: milestoneId }),
    }),

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

  // webhooks
  listWebhooks: (owner: string, name: string) =>
    req<Webhook[]>(`/users/${owner}/repos/${name}/webhooks`),
  createWebhook: (owner: string, name: string, url: string, secret?: string) =>
    req<Webhook>(`/users/${owner}/repos/${name}/webhooks`, {
      method: "POST",
      body: JSON.stringify({ url, secret: secret ?? "" }),
    }),
  deleteWebhook: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/webhooks/${id}`, { method: "DELETE" }),

  // pipeline（CI）
  getPipeline: (owner: string, name: string) =>
    req<{ enabled: boolean; file: string }>(`/users/${owner}/repos/${name}/pipeline`),
  setPipeline: (owner: string, name: string, enabled: boolean) =>
    req<{ enabled: boolean }>(`/users/${owner}/repos/${name}/pipeline`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  listPipelineRuns: (owner: string, name: string) =>
    req<PipelineRun[]>(`/users/${owner}/repos/${name}/pipeline/runs?limit=50`),
  getPipelineRun: (owner: string, name: string, id: number) =>
    req<PipelineRun>(`/users/${owner}/repos/${name}/pipeline/runs/${id}`),
  triggerPipelineRun: (owner: string, name: string, ref?: string) =>
    req<PipelineRun>(`/users/${owner}/repos/${name}/pipeline/runs`, {
      method: "POST",
      body: JSON.stringify(ref ? { ref } : {}),
    }),

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

  // pull requests
  listPulls: (owner: string, name: string, state?: PullState) =>
    req<PullRequest[]>(
      `/users/${owner}/repos/${name}/pulls${state ? `?state=${state}` : ""}`,
    ),
  createPull: (
    owner: string,
    name: string,
    title: string,
    body: string,
    sourceBranch: string,
    targetBranch: string,
  ) =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls`, {
      method: "POST",
      body: JSON.stringify({ title, body, source_branch: sourceBranch, target_branch: targetBranch }),
    }),
  getPull: (owner: string, name: string, number: number) =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}`),
  pullDiff: (owner: string, name: string, number: number) =>
    req<PullDiff>(`/users/${owner}/repos/${name}/pulls/${number}/diff`),
  mergePull: (owner: string, name: string, number: number, method?: "merge" | "squash") =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}/merge`, {
      method: "POST",
      body: JSON.stringify(method ? { method } : {}),
    }),
  setPullState: (owner: string, name: string, number: number, state: "open" | "closed") =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}/state`, {
      method: "POST",
      body: JSON.stringify({ state }),
    }),

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

  // ssh keys
  listKeys: () => req<SSHKey[]>("/keys"),
  createKey: (name: string, publicKey: string) =>
    req<SSHKey>("/keys", { method: "POST", body: JSON.stringify({ name, public_key: publicKey }) }),
  deleteKey: (id: number) => req<null>(`/keys/${id}`, { method: "DELETE" }),

  // gpg keys（提交签名验证）
  listGPGKeys: () => req<GPGKey[]>("/gpg"),
  addGPGKey: (armor: string) =>
    req<GPGKey>("/gpg", { method: "POST", body: JSON.stringify({ armor }) }),
  deleteGPGKey: (id: number) => req<null>(`/gpg/${id}`, { method: "DELETE" }),

  // personal access tokens
  listPATs: () => req<PAT[]>("/tokens"),
  createPAT: (name: string, scopes: string[]) =>
    req<CreatedPAT>("/tokens", { method: "POST", body: JSON.stringify({ name, scopes }) }),
  deletePAT: (id: number) => req<null>(`/tokens/${id}`, { method: "DELETE" }),
};
