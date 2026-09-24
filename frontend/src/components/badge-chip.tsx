import { Award } from "lucide-react";
import { badgeImageUrl, type Badge } from "@/lib/api";
import { cn } from "@/lib/utils";

/** 单个徽章：有图标显示图标，其次 emoji 兜底，最后显示奖章；悬停显示描述。 */
export function BadgeChip({ badge, className }: { badge: Badge; className?: string }) {
  const url = badgeImageUrl(badge);
  return (
    <span
      title={badge.description || badge.label}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border bg-background px-2 py-0.5 text-xs font-medium",
        className,
      )}
    >
      {url ? (
        <img src={url} alt="" className="h-3.5 w-3.5 shrink-0 rounded-full object-cover" />
      ) : badge.emoji ? (
        <span aria-hidden className="shrink-0 text-sm leading-none">
          {badge.emoji}
        </span>
      ) : (
        <Award className="h-3.5 w-3.5 shrink-0 text-amber-500" />
      )}
      <span className="max-w-40 truncate">{badge.label}</span>
    </span>
  );
}
