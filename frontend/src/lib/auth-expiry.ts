/**
 * 会话过期(401)处理：api 层回调通知 App 切回登录页，并暂存当前路径供登录后回跳。
 * sessionStorage 键："gitdash:return_to"
 */

type Handler = () => void;
let handler: Handler | null = null;

/** App 挂载时注册：收到过期通知后切换到登录页。 */
export function onAuthExpired(fn: Handler): void {
  handler = fn;
}

/** api 层在 401 时调用。 */
export function authExpired(): void {
  handler?.();
}

/** 仅站内相对路径，防开放重定向。 */
function safePath(p: string | null): string | null {
  return p && p.startsWith("/") && !p.startsWith("//") ? p : null;
}

/** 记录当前地址，登录成功后回去。 */
export function stashReturnPath(): void {
  try {
    sessionStorage.setItem(
      "gitdash:return_to",
      window.location.pathname + window.location.search + window.location.hash,
    );
  } catch {
    /* ignore */
  }
}

/** 取出并清除暂存地址；优先级：URL ?redirect= > 暂存值 > null。 */
export function takeReturnPath(searchParam: string | null): string {
  let p: string | null = safePath(searchParam);
  if (!p) {
    try {
      p = safePath(sessionStorage.getItem("gitdash:return_to"));
    } catch {
      /* ignore */
    }
  }
  clearReturnPath();
  return p ?? "/";
}

/** 主动登出时清掉暂存地址，避免下次登录回跳到过期页面。 */
export function clearReturnPath(): void {
  try {
    sessionStorage.removeItem("gitdash:return_to");
  } catch {
    /* ignore */
  }
}
