import { useEffect, useState } from "react";
import { cn, formatDate, formatRelativeTime } from "@/lib/utils";

// 所有 RelativeTime 实例共享一个定时器，每分钟刷新一次，避免每处各起 interval。
let now = Date.now();
const listeners = new Set<() => void>();
let timer: ReturnType<typeof setInterval> | undefined;

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  if (timer === undefined) {
    timer = setInterval(() => {
      now = Date.now();
      listeners.forEach((l) => l());
    }, 60_000);
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && timer !== undefined) {
      clearInterval(timer);
      timer = undefined;
    }
  };
}

/** 订阅共享的“当前时间”，分钟级刷新。 */
function useNow(): number {
  const [, force] = useState(0);
  useEffect(() => subscribe(() => force((n) => n + 1)), []);
  return now;
}

/**
 * 人性化时间戳：近期显示相对时间（“3 分钟前”），鼠标悬停显示精确时间。
 * 超过 30 天自动回退为绝对日期。
 */
export function RelativeTime({
  iso,
  locale,
  className,
}: {
  iso: string | null | undefined;
  /** BCP-47 区域标记，来自 dateLocale(lang) */
  locale: string;
  className?: string;
}) {
  const current = useNow();
  if (!iso) return null;
  return (
    <time
      dateTime={iso}
      title={formatDate(iso, locale)}
      className={cn("whitespace-nowrap", className)}
    >
      {formatRelativeTime(iso, locale, current)}
    </time>
  );
}
