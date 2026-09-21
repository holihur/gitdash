import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(iso: string, locale?: string): string {
  try {
    return new Date(iso).toLocaleString(locale);
  } catch {
    return iso;
  }
}

const MINUTE_MS = 60_000;
const HOUR_MS = 3_600_000;
const DAY_MS = 86_400_000;
// 超过该跨度后不再展示相对时间，直接给出精确日期，避免出现“8 个月前”这类
// 模糊且无意义的描述。
const RELATIVE_LIMIT_DAYS = 30;

/**
 * 人类可读的时间：近期显示“3 分钟前 / 昨天 / 5 天前”，超过 30 天回退到
 * 本地化的绝对时间。使用 Intl.RelativeTimeFormat，自动适配当前语言。
 *
 * `now` 可注入以便测试与定时刷新。
 */
export function formatRelativeTime(
  iso: string,
  locale?: string,
  now: number = Date.now(),
): string {
  const time = new Date(iso).getTime();
  if (Number.isNaN(time)) return iso;

  let rtf: Intl.RelativeTimeFormat;
  try {
    rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  } catch {
    return formatDate(iso, locale);
  }

  const delta = time - now;
  const abs = Math.abs(delta);
  if (abs < 45_000) return rtf.format(0, "second");

  const minutes = Math.round(delta / MINUTE_MS);
  if (Math.abs(minutes) < 60) return rtf.format(minutes, "minute");

  const hours = Math.round(delta / HOUR_MS);
  if (Math.abs(hours) < 24) return rtf.format(hours, "hour");

  const days = Math.round(delta / DAY_MS);
  if (Math.abs(days) < RELATIVE_LIMIT_DAYS) return rtf.format(days, "day");

  return formatDate(iso, locale);
}

export function formatSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

/**
 * 复制文本到剪贴板。
 *
 * 优先使用异步 Clipboard API；它只在安全上下文（https / localhost）且
 * 用户手势内可用，明文 http 的 LAN 访问下 `navigator.clipboard` 为 undefined，
 * 部分移动端即使有 API 也会 reject，因此失败后仍要回退到
 * hidden textarea / 选区 + execCommand("copy")。
 */
export async function copyText(text: string): Promise<void> {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // 继续尝试兼容路径（http / 权限受限 / 移动端）。
    }
  }
  if (legacyCopyWithTextarea(text) || legacyCopyWithRange(text)) return;
  throw new Error("copy failed");
}

/** 隐藏 textarea + execCommand；大多数桌面 / Android 浏览器适用。 */
function legacyCopyWithTextarea(text: string): boolean {
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.setAttribute("aria-hidden", "true");
  ta.tabIndex = -1;
  // 保持在视口内但不可见：部分移动端浏览器对移出视口的元素调 select() 会失败。
  ta.style.position = "fixed";
  ta.style.top = "0";
  ta.style.left = "0";
  ta.style.width = "1px";
  ta.style.height = "1px";
  ta.style.padding = "0";
  ta.style.border = "none";
  ta.style.outline = "none";
  ta.style.boxShadow = "none";
  ta.style.background = "transparent";
  ta.style.opacity = "0";
  ta.style.fontSize = "16px"; // 避免 iOS 聚焦时自动放大
  document.body.appendChild(ta);

  const selection = document.getSelection();
  const previous =
    selection && selection.rangeCount > 0 ? selection.getRangeAt(0).cloneRange() : null;

  let ok = false;
  try {
    ta.focus();
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  } finally {
    document.body.removeChild(ta);
    if (previous && selection) {
      selection.removeAllRanges();
      selection.addRange(previous);
    }
  }
  return ok;
}

/** 基于 DOM Range 的选区复制；用于 iOS Safari 下 readonly textarea 选不中的情况。 */
function legacyCopyWithRange(text: string): boolean {
  const host = document.createElement("div");
  host.textContent = text;
  // iOS 要求选区位于可编辑元素内，execCommand("copy") 才会生效。
  host.contentEditable = "true";
  host.setAttribute("aria-hidden", "true");
  host.tabIndex = -1;
  host.style.position = "fixed";
  host.style.top = "0";
  host.style.left = "0";
  host.style.width = "1px";
  host.style.height = "1px";
  host.style.overflow = "hidden";
  host.style.opacity = "0";
  host.style.whiteSpace = "pre-wrap";
  host.style.fontSize = "16px"; // 避免 iOS 聚焦时自动放大
  document.body.appendChild(host);

  const selection = document.getSelection();
  const previous =
    selection && selection.rangeCount > 0 ? selection.getRangeAt(0).cloneRange() : null;

  let ok = false;
  try {
    host.focus();
    const range = document.createRange();
    range.selectNodeContents(host);
    selection?.removeAllRanges();
    selection?.addRange(range);
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  } finally {
    document.body.removeChild(host);
    if (previous && selection) {
      selection.removeAllRanges();
      selection.addRange(previous);
    }
  }
  return ok;
}
