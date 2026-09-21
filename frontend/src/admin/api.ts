import { toast } from "sonner";
import { apiErrorMsg } from "@/lib/errors";

export async function adminReq<T>(path: string, body?: unknown, method?: string): Promise<T> {
  const opts: RequestInit = {
    method: body === undefined ? method ?? "GET" : method ?? "POST",
    credentials: "same-origin",
    headers: {},
  };
  if (body !== undefined) {
    opts.headers = { "Content-Type": "application/json" };
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`/api/admin${path}`, opts);
  if (res.status === 404) throw new ApiDisabledError();
  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    /* ignore */
  }
  if (!res.ok) {
    const msg = (data as { error?: string })?.error ?? res.statusText;
    throw new Error(msg);
  }
  return data as T;
}

export async function adminList<T>(path: string): Promise<{ items: T; total: number }> {
  const res = await fetch(`/api/admin${path}`, { credentials: "same-origin" });
  if (res.status === 404) throw new ApiDisabledError();
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const data = await res.json();
      msg = (data as { error?: string })?.error ?? msg;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  const total = Number(res.headers.get("X-Total-Count") ?? "0");
  return { items: (await res.json()) as T, total: Number.isFinite(total) ? total : 0 };
}

export type Translator = (key: string, vars?: Record<string, string | number>) => string | undefined;

/** 返回原始响应与解析后的错误体（含 code），供用户管理接口按错误码做 i18n。 */
export async function adminUserReq(
  path: string,
  method: string,
  body?: unknown,
): Promise<
  | { ok: true; status: number }
  | { ok: false; status: number; code?: string; error?: string }
> {
  const opts: RequestInit = { method, credentials: "same-origin", headers: {} };
  if (body !== undefined) {
    opts.headers = { "Content-Type": "application/json" };
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`/api/admin${path}`, opts);
  let data: { error?: string; code?: string } | null = null;
  try {
    data = await res.json();
  } catch {
    /* ignore */
  }
  if (!res.ok) return { ok: false, status: res.status, code: data?.code, error: data?.error ?? res.statusText };
  return { ok: true, status: res.status };
}

export function userActionError(
  to: Translator,
  r: { ok: false; status: number; code?: string; error?: string },
  fallbackKey: string,
): string {
  if (r.status === 409) return to("admin.userExists") ?? r.error ?? "conflict";
  if (r.code === "weak_password" || r.status === 400) return to("admin.weakPassword") ?? r.error ?? "bad request";
  if (r.status === 404) return to("admin.userNotFound") ?? r.error ?? "not found";
  return r.error ?? to(fallbackKey) ?? fallbackKey;
}

export class ApiDisabledError extends Error {
  constructor() {
    super("admin_disabled");
    this.name = "ApiDisabledError";
  }
}

export interface Settings {
  github_oauth_enabled: boolean;
  github_client_id: string;
  github_has_secret: boolean;
  google_oauth_enabled: boolean;
  google_client_id: string;
  google_has_secret: boolean;
  oidc_enabled: boolean;
  oidc_name: string;
  oidc_issuer: string;
  oidc_client_id: string;
  oidc_has_secret: boolean;
  smtp_enabled: boolean;
  smtp_host: string;
  smtp_port: string;
  smtp_user: string;
  smtp_from: string;
  smtp_has_pass: boolean;
  feedback_enabled: boolean;
  feedback_repo: string;
  feedback_has_token: boolean;
  feedback_local: boolean;
  gitlab_enabled: boolean;
  gitlab_client_id: string;
  gitlab_has_secret: boolean;
  gitlab_base_url: string;
  gitea_enabled: boolean;
  gitea_client_id: string;
  gitea_has_secret: boolean;
  gitea_base_url: string;
  bitbucket_enabled: boolean;
  bitbucket_client_id: string;
  bitbucket_has_secret: boolean;
  docs_url: string;
  /** 账号密码登录（默认开启，管理端可关闭）。 */
  password_login_enabled: boolean;
  /** 匿名可访问的 Swagger/OpenAPI 文档（默认开启，管理端可关闭）。 */
  swagger_enabled: boolean;
}

export function toastError(to: (k: string, v?: Record<string, string | number>) => string | undefined, e: unknown) {
  toast.error(apiErrorMsg(to, e));
}

