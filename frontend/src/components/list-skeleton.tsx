import { Skeleton } from "@/components/ui/skeleton";

/** 列表加载骨架屏：n 行圆角条目 + 顶部分类条，与列表卡片布局对齐。 */
export default function ListSkeleton({ rows = 5, header = true }: { rows?: number; header?: boolean }) {
  return (
    <div className="space-y-2" aria-busy="true" aria-live="polite">
      {header && <Skeleton className="h-5 w-40" />}
      <div className="divide-y divide-border overflow-hidden rounded-lg border bg-card">
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} className="flex items-center gap-3 px-4 py-3">
            <Skeleton className="h-4 w-4 shrink-0 rounded-full" />
            <div className="min-w-0 flex-1 space-y-1.5">
              <Skeleton className="h-4" style={{ width: `${55 + ((i * 13) % 35)}%` }} />
              <Skeleton className="h-3 w-1/3" />
            </div>
            <Skeleton className="h-8 w-16 shrink-0" />
          </div>
        ))}
      </div>
    </div>
  );
}
