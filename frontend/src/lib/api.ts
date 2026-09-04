const TOKEN_KEY = "gitdash-token";

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
}

export interface Repo {
  id: number;
  owner: string;
  name: string;
  description: string;
  created_at: string;
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
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
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
    req<{ token: string; username: string }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  logout: () => req<null>("/auth/logout", { method: "POST" }),
  me: () => req<User>("/me"),

  // repos（owner 用于展示/校验，URL 由会话中的用户决定）
  listRepos: () => req<Repo[]>("/repos"),
  createRepo: (name: string, description: string) =>
    req<Repo>("/repos", { method: "POST", body: JSON.stringify({ name, description }) }),
  getRepo: (_owner: string, name: string) => req<Repo>(`/repos/${name}`),
  deleteRepo: (_owner: string, name: string) => req<null>(`/repos/${name}`, { method: "DELETE" }),

  // git browsing
  branches: (_owner: string, name: string) => req<Branch[]>(`/repos/${name}/branches`),
  tree: (_owner: string, name: string, ref: string, path: string) =>
    req<{ path: string; entries: TreeEntry[] }>(
      `/repos/${name}/tree?ref=${encodeURIComponent(ref)}&path=${encodeURIComponent(path)}`,
    ),
  blob: (_owner: string, name: string, ref: string, path: string) =>
    req<Blob>(
      `/repos/${name}/blob?ref=${encodeURIComponent(ref)}&path=${encodeURIComponent(path)}`,
    ),
  commits: (_owner: string, name: string, ref: string) =>
    req<Commit[]>(`/repos/${name}/commits?ref=${encodeURIComponent(ref)}`),

  // issues
  listIssues: (_owner: string, name: string) => req<Issue[]>(`/repos/${name}/issues`),
  createIssue: (_owner: string, name: string, title: string, body: string) =>
    req<Issue>(`/repos/${name}/issues`, {
      method: "POST",
      body: JSON.stringify({ title, body }),
    }),
  setIssueState: (_owner: string, name: string, number: number, state: "open" | "closed") =>
    req<Issue>(`/repos/${name}/issues/${number}`, {
      method: "PATCH",
      body: JSON.stringify({ state }),
    }),

  // ssh keys
  listKeys: () => req<SSHKey[]>("/keys"),
  createKey: (name: string, publicKey: string) =>
    req<SSHKey>("/keys", { method: "POST", body: JSON.stringify({ name, public_key: publicKey }) }),
  deleteKey: (id: number) => req<null>(`/keys/${id}`, { method: "DELETE" }),
};
