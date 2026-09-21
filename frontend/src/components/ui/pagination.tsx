import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export const PAGE_SIZES = [10, 20, 50] as const;

interface PaginationProps {
  page: number; // 从 1 开始
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  className?: string;
}

function pageList(page: number, pageCount: number): (number | "…")[] {
  if (pageCount <= 7) return Array.from({ length: pageCount }, (_, i) => i + 1);
  const pages: (number | "…")[] = [1];
  const from = Math.max(2, page - 1);
  const to = Math.min(pageCount - 1, page + 1);
  if (from > 2) pages.push("…");
  for (let i = from; i <= to; i++) pages.push(i);
  if (to < pageCount - 1) pages.push("…");
  pages.push(pageCount);
  return pages;
}

export default function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  className,
}: PaginationProps) {
  const { t } = useI18n();
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const current = Math.min(page, pageCount);
  if (total === 0) return null;

  return (
    <div className={cn("flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between", className)}>
      <p className="text-sm text-muted-foreground">
        {t("pagination.total", { n: total })}
      </p>
      <div className="flex items-center justify-between gap-2 sm:justify-end">
        <select
          value={pageSize}
          onChange={(e) => onPageSizeChange(Number(e.target.value))}
          className="h-9 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring sm:h-8"
          title={t("pagination.perPage")}
        >
          {PAGE_SIZES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <div className="flex items-center gap-1">
          <Button
            variant="outline"
            size="sm"
            className="h-9 gap-1 px-2 sm:h-8"
            disabled={current <= 1}
            onClick={() => onPageChange(current - 1)}
            aria-label={t("pagination.prev")}
          >
            <ChevronLeft className="h-4 w-4" />
            <span className="hidden sm:inline">{t("pagination.prev")}</span>
          </Button>
          {/* 手机端：页码列表易溢出，改为“当前 / 总数”紧凑指示 */}
          <span className="min-w-[3.5rem] px-1 text-center text-sm text-muted-foreground sm:hidden">
            {current} / {pageCount}
          </span>
          <div className="hidden items-center gap-1 sm:flex">
            {pageList(current, pageCount).map((p, i) =>
              p === "…" ? (
                <span key={`ellipsis-${i}`} className="px-1 text-sm text-muted-foreground">
                  …
                </span>
              ) : (
                <Button
                  key={p}
                  variant={p === current ? "default" : "outline"}
                  size="sm"
                  className="h-8 min-w-8 px-2"
                  onClick={() => onPageChange(p)}
                >
                  {p}
                </Button>
              ),
            )}
          </div>
          <Button
            variant="outline"
            size="sm"
            className="h-9 gap-1 px-2 sm:h-8"
            disabled={current >= pageCount}
            onClick={() => onPageChange(current + 1)}
            aria-label={t("pagination.next")}
          >
            <span className="hidden sm:inline">{t("pagination.next")}</span>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
