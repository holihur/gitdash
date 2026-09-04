const TOKEN_KEY = "gitdash-token";

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
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export interface User {
  username: string;
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
  /** 仅“可访问仓库列表”返回：owner / read / write */
  role?: "owner" | "read" | "write";
}

export interface SSHKey {
  id: number;
  name: string;
  public_key: string;
  fingerprint: string;
  created_at: string;
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
}

export interface Blob {
  path: string;
  size: number;
  encoding: "utf-8" | "binary" | "truncated";
  content: string;
}

export interface Commit {
  sha: string;
  author: string;
  date: string;
  message: string;
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
}

export interface Collab {
  owner: string;
  repo: string;
  username: string;
  permission: "read" | "write";
  created_at: string;
}

export interface Webhook {
  id: number;
  owner: string;
  repo: string;
  url: string;
  created_at: string;
}
export function cloneUrl(owner: string, name: string): string {
  return `ssh://git@${window.location.hostname}:2222/${owner}/${name}.git`;
}

export function cloneCommand(owner: string, name: string): string {
  return `git clone ${cloneUrl(owner, name)}`;
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const token = getToken();
  const res = await fetch(`/api${path}`, {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(opts.headers ?? {}),
    },
  });
  if (!res.ok) {
    if (res.status === 401 && window.location.pathname !== "/login") {
      // 会话失效：清除并回登录页
      clearToken();
      window.location.href = "/login";
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
  createRepo: (name: string, description: string) =>
    req<Repo>("/repos", { method: "POST", body: JSON.stringify({ name, description }) }),
  getRepo: (owner: string, name: string) => req<Repo>(`/users/${owner}/repos/${name}`),
  deleteRepo: (owner: string, name: string) =>
    req<null>(`/users/${owner}/repos/${name}`, { method: "DELETE" }),

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
  createWebhook: (owner: string, name: string, url: string) =>
    req<Webhook>(`/users/${owner}/repos/${name}/webhooks`, {
      method: "POST",
      body: JSON.stringify({ url }),
    }),
  deleteWebhook: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/webhooks/${id}`, { method: "DELETE" }),

  // ssh keys
  listKeys: () => req<SSHKey[]>("/keys"),
  createKey: (name: string, publicKey: string) =>
    req<SSHKey>("/keys", { method: "POST", body: JSON.stringify({ name, public_key: publicKey }) }),
  deleteKey: (id: number) => req<null>(`/keys/${id}`, { method: "DELETE" }),
};
