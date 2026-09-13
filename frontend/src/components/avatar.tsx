import { useState } from "react";
import { cn } from "@/lib/utils";

/**
 * 用户头像：有头像时展示图片，加载失败（未设置头像）时回退到用户名首字母。
 * 未显式提供 src 时默认请求 /api/users/{username}/avatar。
 */
export function Avatar({
  username,
  src,
  size = 32,
  className,
  version,
}: {
  username: string;
  src?: string;
  size?: number;
  className?: string;
  /** 追加 ?v= 破缓存（上传/删除头像后刷新） */
  version?: number | string;
}) {
  const [failed, setFailed] = useState(false);
  const base = src ?? `/api/users/${encodeURIComponent(username)}/avatar`;
  const url = version !== undefined && version !== "" ? `${base}?v=${version}` : base;
  const initial = (username || "?").slice(0, 1).toUpperCase();

  if (failed) {
    return (
      <span
        aria-hidden="true"
        className={cn(
          "flex shrink-0 items-center justify-center rounded-full bg-muted font-semibold text-muted-foreground",
          className,
        )}
        style={{ width: size, height: size, fontSize: Math.round(size * 0.45) }}
      >
        {initial}
      </span>
    );
  }
  return (
    <img
      src={url}
      alt={username}
      width={size}
      height={size}
      loading="lazy"
      onError={() => setFailed(true)}
      className={cn("shrink-0 rounded-full object-cover", className)}
    />
  );
}
