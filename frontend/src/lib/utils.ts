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

export function formatSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

/**
 * copyText 复制文本到剪贴板。
 * Clipboard API 仅在安全上下文（https / localhost）可用；
 * 明文 http 的 LAN IP 访问下 navigator.clipboard 为 undefined，
 * 此时回退到隐藏 textarea + execCommand("copy")。
 */
export function copyText(text: string): Promise<void> {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(text);
  }
  return new Promise((resolve, reject) => {
    const ta = document.createElement("textarea");
    ta.value = text;
    // 防止页面滚动/闪烁：移出视口但保持可选中
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.setAttribute("readonly", "");
    document.body.appendChild(ta);
    try {
      ta.select();
      const ok = document.execCommand("copy");
      if (ok) {
        resolve();
      } else {
        reject(new Error("copy failed"));
      }
    } catch (e) {
      reject(e instanceof Error ? e : new Error(String(e)));
    } finally {
      document.body.removeChild(ta);
    }
  });
}
