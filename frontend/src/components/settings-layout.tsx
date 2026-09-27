import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export interface SettingsNavItem {
  key: string;
  label: string;
  icon?: LucideIcon;
  content: ReactNode;
}

/**
 * 设置页通用布局：左侧分区菜单（窄屏横向滚动）+ 右侧内容。
 * 菜单选中项由调用方通过路由维护（active + onSelect），实现 URL 深链与前进后退。
 */
export function SettingsLayout({
  title,
  description,
  actions,
  items,
  active,
  onSelect,
}: {
  title?: string;
  description?: string;
  actions?: ReactNode;
  items: SettingsNavItem[];
  active: string;
  onSelect: (key: string) => void;
}) {
  const current = items.find((i) => i.key === active) ?? items[0];
  return (
    <div className="mx-auto max-w-5xl">
      {(title || description || actions) && (
        <div className="mb-4 flex items-start justify-between gap-3">
          <div className="min-w-0">
            {title && <h1 className="truncate text-2xl font-bold">{title}</h1>}
            {description && (
              <p className="text-sm text-muted-foreground">{description}</p>
            )}
          </div>
          {actions}
        </div>
      )}
      <div className="flex flex-col gap-6 lg:flex-row">
        <nav aria-label={title} className="lg:shrink-0">
          <ul
            role="tablist"
            className="flex gap-1 overflow-x-auto pb-1 lg:sticky lg:top-4 lg:w-56 lg:flex-col lg:overflow-visible"
          >
            {items.map((it) => {
              const Icon = it.icon;
              const isActive = it.key === current?.key;
              return (
                <li key={it.key}>
                  <button
                    type="button"
                    role="tab"
                    aria-selected={isActive}
                    onClick={() => onSelect(it.key)}
                    className={cn(
                      "flex w-full items-center gap-2 whitespace-nowrap rounded-md px-3 py-2 text-sm transition-colors",
                      isActive
                        ? "bg-muted font-medium text-foreground"
                        : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                    )}
                  >
                    {Icon && <Icon className="h-4 w-4 shrink-0" />}
                    {it.label}
                  </button>
                </li>
              );
            })}
          </ul>
        </nav>
        <div className="min-w-0 flex-1 space-y-4">{current?.content}</div>
      </div>
    </div>
  );
}
