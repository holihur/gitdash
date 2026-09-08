// 会话通过 httpOnly Cookie(gitdash_session) 自动携带，前端不再持有 token。

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

/** 启动时调用：拉取实例信息（真实 SSH 端口），失败保持默认 2222。 */
export async function loadInstanceInfo(): Promise<void> {
  try {
    const r = await req<{ version: string; ssh_port: string }>("/instance");
    if (r.ssh_port) sshPort = r.ssh_port;
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
  return res;
}

export async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await send(path, opts);
  if (res.status === 204) return null as T;
  return res.json();
}

/** multipart 上传：不设 Content-Type（由浏览器带 boundary） */
export async function sendForm<T>(path: string, form: FormData): Promise<T> {
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
}

// 分页列表请求：数组 JSON + X-Total-Count 响应头
export interface Paged<T> {
  items: T;
  total: number;
}

export async function reqPage<T>(path: string, opts: RequestInit = {}): Promise<Paged<T>> {
  const res = await send(path, opts);
  const items = (await res.json()) as T;
  const total = Number(res.headers.get("X-Total-Count") ?? 0);
  return { items, total: Number.isNaN(total) ? 0 : total };
}

export function pageQuery(limit?: number, offset?: number): string {
  const p = new URLSearchParams();
  if (limit != null) p.set("limit", String(limit));
  if (offset != null) p.set("offset", String(offset));
  const q = p.toString();
  return q ? `?${q}` : "";
}
