import { useRef, type ReactNode } from "react";
import { Plus } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { ProjectCard } from "@/lib/api";
import { Button } from "@/components/ui/button";

export function VirtualCardList({ cards, renderCard }: { cards: ProjectCard[]; renderCard: (card: ProjectCard) => ReactNode }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: cards.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 72,
    overscan: 8,
  });

  return (
    <div ref={scrollRef} className="max-h-[45vh] overflow-y-auto px-2">
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((vi) => {
          const card = cards[vi.index];
          return (
            <div
              key={card.id}
              data-index={vi.index}
              ref={virtualizer.measureElement}
              className="pb-2"
              style={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${vi.start}px)` }}
            >
              {renderCard(card)}
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** 单元格内的“添加卡片”入口：点击后由看板唤起新建卡片对话框。 */
export function AddCardCell({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <Button
      variant="ghost"
      size="sm"
      className="h-7 w-full justify-center gap-1 text-xs text-muted-foreground"
      onClick={onClick}
    >
      <Plus className="h-3.5 w-3.5" />
      {label}
    </Button>
  );
}
