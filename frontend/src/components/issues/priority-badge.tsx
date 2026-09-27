import { AlertOctagon, AlertTriangle, ArrowDown, ArrowUp } from "lucide-react";
import type { IssuePriority } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** 允许的优先级取值（与后端 normalizeIssuePriority 一致）。 */
export const ISSUE_PRIORITIES: IssuePriority[] = ["critical", "high", "medium", "low"];

/** 优先级 → i18n key（如 issues.priorityHigh）。 */
export function priorityI18nKey(p?: string): string | null {
  switch (p) {
    case "critical":
      return "issues.priorityCritical";
    case "high":
      return "issues.priorityHigh";
    case "medium":
      return "issues.priorityMedium";
    case "low":
      return "issues.priorityLow";
    default:
      return null;
  }
}

const PRIORITY_CLASS: Record<IssuePriority, string> = {
  critical: "border-red-500/50 bg-red-500/10 text-red-600 dark:text-red-400",
  high: "border-orange-500/50 bg-orange-500/10 text-orange-600 dark:text-orange-400",
  medium: "border-amber-500/50 bg-amber-500/10 text-amber-600 dark:text-amber-400",
  low: "border-sky-500/50 bg-sky-500/10 text-sky-600 dark:text-sky-400",
};

function PriorityIcon({ priority, className }: { priority: IssuePriority; className?: string }) {
  switch (priority) {
    case "critical":
      return <AlertOctagon className={className} />;
    case "high":
      return <ArrowUp className={className} />;
    case "medium":
      return <AlertTriangle className={className} />;
    case "low":
      return <ArrowDown className={className} />;
  }
}

/** 优先级徽标；优先级为空时不渲染任何内容。 */
export function PriorityBadge({ priority, className }: { priority?: string; className?: string }) {
  const { t } = useI18n();
  const key = priorityI18nKey(priority);
  if (!key || !priority) return null;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
        PRIORITY_CLASS[priority as IssuePriority],
        className,
      )}
    >
      <PriorityIcon priority={priority as IssuePriority} className="h-3 w-3" />
      {t(key)}
    </span>
  );
}
