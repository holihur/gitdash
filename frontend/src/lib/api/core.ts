// 会话通过 httpOnly Cookie(gitdash_session) 自动携带，前端不再持有 token。

import { authExpired, stashReturnPath } from "@/lib/auth-expiry";
import { setDocsURL } from "@/lib/docs";

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

let sshPort = "2222";

/** 启动时调用：拉取实例信息（真实 SSH 端口、文档站地址），失败保持默认。 */
export async function loadInstanceInfo(): Promise<void> {
  try {
    const r = await req<{ version: string; ssh_port: string; docs_url?: string }>("/instance");
    if (r.ssh_port) sshPort = r.ssh_port;
    if (r.docs_url) setDocsURL(r.docs_url);
  } catch {
    /* ignore：回退默认端口 */
  }
}

export function cloneUrl(owner: string, name: string): string {
  return `ssh://git@${window.location.hostname}:${sshPort}/${owner}/${name}.git`;
}

export function cloneCommand(owner: string, name: string): string {
  return `git clone ${cloneUrl(owner, name)}`;
}

export async function send(path: string, opts: RequestInit = {}): Promise<Response> {
  const res = await fetch(`/api${path}`, {
    ...opts,
    credentials: "same-origin", // 携带 httpOnly cookie
    headers: {
      "Content-Type": "application/json",
      ...(opts.headers ?? {}),
    },
  });
  if (!res.ok) {
    if (
      res.status === 401 &&
      !path.startsWith("/auth/") && // 登录/注册自身的 401 属于业务错误，不算会话过期
      !path.startsWith("/instance") &&
      !path.startsWith("/version") &&
      !path.startsWith("/auth/providers") &&
      !path.startsWith("/health")
    ) {
      // 会话失效：通知 App 切回登录页并记住当前地址，登录后原路返回（不整页刷新）
      stashReturnPath();
      authExpired();
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
  return res;
}

// ---- 客户端 JSON 缓存（GET） ----
//
// 目标：减少重复请求。相同 GET 在 TTL 内复用结果；并发的相同 GET 共享同一个
// in-flight Promise（StrictMode 开发模式双调用、同页多组件重复拉取都受益）。
// 任意写请求会清空缓存（含进行中的请求的结果，用 generation 防止写回旧数据），
// 避免读到陈旧数据。轮询/手动刷新等需要实时的请求传 { fresh: true } 绕过缓存。
const JSON_CACHE_TTL_MS = 3000;
const jsonCache = new Map<string, { at: number; data: unknown }>();
const inflight = new Map<string, Promise<unknown>>();
// 每次写请求自增：in-flight 的 GET 只有在 generation 未变时才写回缓存。
let cacheGeneration = 0;

export interface CacheOptions {
  /** true 时跳过缓存读取、强制请求并刷新缓存（用于轮询 / 手动刷新） */
  fresh?: boolean;
}

function clearJsonCache(): void {
  jsonCache.clear();
  cacheGeneration++;
}

/** 清空客户端 GET 缓存（测试隔离、或需要强制全量刷新时使用）。 */
export function clearApiCache(): void {
  clearJsonCache();
}

function isMutation(opts: RequestInit): boolean {
  const m = (opts.method ?? "GET").toUpperCase();
  return m !== "GET" && m !== "HEAD";
}

async function cachedJson<T>(
  path: string,
  cache: CacheOptions | undefined,
  loader: () => Promise<T>,
): Promise<T> {
  if (cache?.fresh) {
    const gen = cacheGeneration;
    const data = await loader();
    if (gen === cacheGeneration) jsonCache.set(path, { at: Date.now(), data });
    return data;
  }
  const hit = jsonCache.get(path);
  if (hit && Date.now() - hit.at < JSON_CACHE_TTL_MS) return hit.data as T;
  const pending = inflight.get(path) as Promise<T> | undefined;
  if (pending) return pending;
  const gen = cacheGeneration;
  const p = loader().then((data) => {
    if (gen === cacheGeneration) jsonCache.set(path, { at: Date.now(), data });
    return data;
  });
  inflight.set(path, p);
  try {
    return await p;
  } finally {
    inflight.delete(path);
  }
}

export async function req<T>(path: string, opts: RequestInit = {}, cache?: CacheOptions): Promise<T> {
  const load = async (): Promise<T> => {
    const res = await send(path, opts);
    if (res.status === 204) return null as T;
    return res.json();
  };
  if (isMutation(opts)) {
    clearJsonCache();
    try {
      return await load();
    } finally {
      clearJsonCache();
    }
  }
  return cachedJson(path, cache, load);
}

/** multipart 上传：不设 Content-Type（由浏览器带 boundary） */
export async function sendForm<T>(path: string, form: FormData): Promise<T> {
  // 上传属于写操作：清空 GET 缓存，避免表单变更后读到旧数据。
  clearJsonCache();
  try {
    const res = await fetch(`/api${path}`, { method: "POST", credentials: "same-origin", body: form });
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const body = await res.json();
        if (typeof body?.error === "string") msg = body.error;
      } catch {
        /* ignore */
      }
      throw new ApiError(res.status, msg);
    }
    return res.json() as Promise<T>;
  } finally {
    clearJsonCache();
  }
}

// 分页列表请求：数组 JSON + X-Total-Count 响应头
export interface Paged<T> {
  items: T;
  total: number;
}

export async function reqPage<T>(
  path: string,
  opts: RequestInit = {},
  cache?: CacheOptions,
): Promise<Paged<T>> {
  const load = async (): Promise<Paged<T>> => {
    const res = await send(path, opts);
    const items = (await res.json()) as T;
    const total = Number(res.headers.get("X-Total-Count") ?? 0);
    return { items, total: Number.isNaN(total) ? 0 : total };
  };
  if (isMutation(opts)) {
    clearJsonCache();
    try {
      return await load();
    } finally {
      clearJsonCache();
    }
  }
  return cachedJson(path, cache, load);
}

export function pageQuery(limit?: number, offset?: number): string {
  const p = new URLSearchParams();
  if (limit != null) p.set("limit", String(limit));
  if (offset != null) p.set("offset", String(offset));
  const q = p.toString();
  return q ? `?${q}` : "";
}
