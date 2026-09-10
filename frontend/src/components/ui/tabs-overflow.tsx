import { useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Check, ChevronDown, MoreHorizontal } from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { TabsList, TabsTrigger } from "@/components/ui/tabs";

export interface OverflowTab {
  value: string;
  label: ReactNode;
}

const GAP = 4; // tab 之间的近似间距
const MORE_WIDTH = 40; // “更多”按钮的近似宽度（图标 + 两侧 padding）

/**
 * 自适应 Tab 列表：空间不足时把溢出的 tab 收进右侧「更多」下拉菜单。
 * 需要配合受控的 <Tabs value onValueChange> 一起使用。
 */
export function TabsListOverflow({
  tabs,
  value,
  onValueChange,
  className,
  listClassName,
}: {
  tabs: OverflowTab[];
  value: string;
  onValueChange: (value: string) => void;
  className?: string;
  listClassName?: string;
}) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const [measured, setMeasured] = useState<{ key: string; widths: number[] } | null>(null);
  const [available, setAvailable] = useState(0);

  // 用「值 + 文案」生成稳定 key，用于判断是否需要重新测量宽度。
  const tabsKey = useMemo(
    () =>
      tabs
        .map((t) => `${t.value}\u0000${typeof t.label === "string" || typeof t.label === "number" ? String(t.label) : ""}`)
        .join("\u0001"),
    [tabs],
  );

  // 首次（或 tabs 变化后）所有 tab 都会渲染出来，此时测量每个 tab 的真实宽度。
  useLayoutEffect(() => {
    itemRefs.current.length = tabs.length;
    const next = Array.from({ length: tabs.length }, (_, i) => itemRefs.current[i]?.offsetWidth ?? 0);
    setMeasured((prev) => (prev && prev.key === tabsKey ? prev : { key: tabsKey, widths: next }));
  }, [tabs, tabsKey]);

  // 测量外层容器（宽度稳定，不随折叠变化）的可用宽度，并减去 TabsList 的 p-1（左右各 4px）。
  useLayoutEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const update = () => setAvailable(Math.max(0, el.clientWidth - 8));
    update();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const widths = measured && measured.key === tabsKey ? measured.widths : null;

  let hiddenFrom: number | null = null;
  if (widths && widths.length === tabs.length && available > 0) {
    const total = widths.reduce((s, w, i) => s + w + (i > 0 ? GAP : 0), 0);
    if (total > available) {
      let used = 0;
      for (let i = 0; i < widths.length; i++) {
        const gap = i > 0 ? GAP : 0;
        // 只要后面还有 tab，就为「更多」按钮预留宽度。
        const reserveMore = i < widths.length - 1 ? MORE_WIDTH + GAP : 0;
        if (used + gap + widths[i] + reserveMore > available) {
          hiddenFrom = i;
          break;
        }
        used += gap + widths[i];
      }
      if (hiddenFrom === null) hiddenFrom = widths.length - 1;
    }
  }

  const visible = hiddenFrom === null ? tabs : tabs.slice(0, hiddenFrom);
  const hidden = hiddenFrom === null ? [] : tabs.slice(hiddenFrom);
  const activeHidden = hidden.some((tab) => tab.value === value);

  return (
    <div ref={wrapRef} className={cn("relative flex items-center gap-1", className)}>
      <TabsList className={cn("min-w-0", listClassName)}>
        {visible.map((tab, i) => (
          <TabsTrigger
            key={tab.value}
            value={tab.value}
            ref={(el) => {
              itemRefs.current[i] = el;
            }}
          >
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>

      {hidden.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              aria-label="More tabs"
              className="relative h-10 shrink-0 gap-1 px-3 text-muted-foreground data-[state=open]:bg-muted data-[state=open]:text-foreground"
            >
              <MoreHorizontal className="h-4 w-4" />
              <ChevronDown className="h-3 w-3 opacity-60" />
              {activeHidden && (
                <span className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-primary" />
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {hidden.map((tab) => (
              <DropdownMenuItem
                key={tab.value}
                onSelect={() => onValueChange(tab.value)}
                className={cn(value === tab.value && "bg-accent text-accent-foreground")}
              >
                {tab.label}
                {value === tab.value && <Check className="ml-auto h-4 w-4" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  );
}
