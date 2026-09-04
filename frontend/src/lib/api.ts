const TOKEN_KEY = "gitdash-token";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? "dev";
}

export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t);
}

export interface Repo {
  id: number;
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

export function cloneUrl(name: string): string {
  return `ssh://git@${window.location.hostname}:2222/${name}.git`;
}

export function cloneCommand(name: string): string {
  return `git clone ${cloneUrl(name)}`;
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${getToken()}`,
      ...(opts.headers ?? {}),
    },
  });
  if (!res.ok) {
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
  listRepos: () => req<Repo[]>("/repos"),
  getRepo: (name: string) => req<Repo>(`/repos/${name}`),
  createRepo: (name: string, description: string) =>
    req<Repo>("/repos", { method: "POST", body: JSON.stringify({ name, description }) }),
  deleteRepo: (name: string) => req<null>(`/repos/${name}`, { method: "DELETE" }),

  listKeys: () => req<SSHKey[]>("/keys"),
  createKey: (name: string, publicKey: string) =>
    req<SSHKey>("/keys", { method: "POST", body: JSON.stringify({ name, public_key: publicKey }) }),
  deleteKey: (id: number) => req<null>(`/keys/${id}`, { method: "DELETE" }),

  branches: (repo: string) => req<Branch[]>(`/repos/${repo}/branches`),
  tree: (repo: string, ref: string, path: string) =>
    req<{ path: string; entries: TreeEntry[] }>(
      `/repos/${repo}/tree?ref=${encodeURIComponent(ref)}&path=${encodeURIComponent(path)}`,
    ),
  blob: (repo: string, ref: string, path: string) =>
    req<Blob>(
      `/repos/${repo}/blob?ref=${encodeURIComponent(ref)}&path=${encodeURIComponent(path)}`,
    ),
  commits: (repo: string, ref: string) =>
    req<Commit[]>(`/repos/${repo}/commits?ref=${encodeURIComponent(ref)}`),
};
