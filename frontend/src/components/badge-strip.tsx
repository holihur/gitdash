import { useEffect, useState } from "react";
import { api, type Badge } from "@/lib/api";
import { cn } from "@/lib/utils";
import { BadgeChip } from "@/components/badge-chip";

/**
 * 某目标（用户 / 仓库 / 组织）公开展示的徽章条。
 * 没有徽章时不渲染任何内容。
 */
export function BadgeStrip({
  kind,
  owner,
  repo,
  className,
}: {
  kind: "user" | "repo" | "org";
  owner: string;
  repo?: string;
  className?: string;
}) {
  const [badges, setBadges] = useState<Badge[] | null>(null);

  useEffect(() => {
    let alive = true;
    setBadges(null);
    // 包一层 Promise，避免某些测试环境只 mock 了部分 API 时同步抛错。
    Promise.resolve()
      .then(() => api.getBadges(kind, owner, repo))
      .then((b) => {
        if (alive) setBadges(b);
      })
      .catch(() => {
        if (alive) setBadges([]);
      });
    return () => {
      alive = false;
    };
  }, [kind, owner, repo]);

  if (!badges || badges.length === 0) return null;
  return (
    <div className={cn("flex flex-wrap items-center gap-1.5", className)}>
      {badges.map((b) => (
        <BadgeChip key={b.id} badge={b} />
      ))}
    </div>
  );
}
