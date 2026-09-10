import { useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { ChevronDown, MoreHorizontal } from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export interface NavOverflowItem {
  key: string;
  to: string;
  icon: ReactNode;
  label: string;
  badge?: ReactNode;
}

const GAP = 4; // 项之间的近似间距
const MORE_WIDTH = 40; // “更多”按钮的近似宽度

/** 判断当前路由是否匹配该项（"/" 精确匹配，其余按前缀匹配）。 */
function isNavActive(pathname: string, to: string): boolean {
  if (to === "/") return pathname === "/";
  return pathname === to || pathname.startsWith(to + "/");
}

/**
 * 自适应顶部导航：空间不足时把放不下的项收进「更多」下拉菜单。
 */
export function NavOverflow({ items, className }: { items: NavOverflowItem[]; className?: string }) {
  const navRef = useRef<HTMLElement>(null);
  const itemRefs = useRef<(HTMLElement | null)[]>([]);
  const [measured, setMeasured] = useState<{ key: string; widths: number[] } | null>(null);
  const [available, setAvailable] = useState(0);
  const { pathname } = useLocation();

  const itemsKey = useMemo(
    () => items.map((i) => `${i.key}\u0000${i.label}\u0000${i.badge ? "1" : "0"}`).join("\u0001"),
    [items],
  );

  useLayoutEffect(() => {
    itemRefs.current.length = items.length;
    const next = Array.from({ length: items.length }, (_, i) => itemRefs.current[i]?.offsetWidth ?? 0);
    setMeasured((prev) => (prev && prev.key === itemsKey ? prev : { key: itemsKey, widths: next }));
  }, [items, itemsKey]);

  useLayoutEffect(() => {
    const el = navRef.current;
    if (!el) return;
    const update = () => setAvailable(el.clientWidth);
    update();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const widths = measured && measured.key === itemsKey ? measured.widths : null;

  let hiddenFrom: number | null = null;
  if (widths && widths.length === items.length && available > 0) {
    const total = widths.reduce((s, w, i) => s + w + (i > 0 ? GAP : 0), 0);
    if (total > available) {
      let used = 0;
      for (let i = 0; i < widths.length; i++) {
        const gap = i > 0 ? GAP : 0;
        // 只要后面还有项，就为「更多」按钮预留宽度。
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

  const visible = hiddenFrom === null ? items : items.slice(0, hiddenFrom);
  const hidden = hiddenFrom === null ? [] : items.slice(hiddenFrom);
  const activeHidden = hidden.some((item) => isNavActive(pathname, item.to));

  return (
    <nav ref={navRef} className={cn("relative flex min-w-0 flex-1 items-center gap-1", className)}>
      {visible.map((item, i) => (
        <Button
          key={item.key}
          asChild
          variant="ghost"
          size="sm"
          ref={(el) => {
            itemRefs.current[i] = el;
          }}
          className={cn("px-2 sm:px-3", item.badge && "relative")}
        >
          <NavLink to={item.to} className="flex items-center gap-2">
            {item.icon}
            <span className="hidden sm:inline">{item.label}</span>
            {item.badge}
          </NavLink>
        </Button>
      ))}

      {hidden.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              aria-label="More"
              className="relative h-9 shrink-0 gap-1 px-2 text-muted-foreground data-[state=open]:bg-muted data-[state=open]:text-foreground"
            >
              <MoreHorizontal className="h-4 w-4" />
              <ChevronDown className="h-3 w-3 opacity-60" />
              {activeHidden && (
                <span className="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-primary" />
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {hidden.map((item) => {
              const active = isNavActive(pathname, item.to);
              return (
                <DropdownMenuItem key={item.key} asChild className={cn(active && "bg-accent text-accent-foreground")}>
                  <NavLink to={item.to}>
                    {item.icon}
                    {item.label}
                  </NavLink>
                </DropdownMenuItem>
              );
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </nav>
  );
}
