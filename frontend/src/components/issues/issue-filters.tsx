import { Search } from "lucide-react";
import type { Label, Milestone } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import LabelChip from "@/components/label-chip";

/** issue 搜索框 + 状态筛选 + 标签/里程碑筛选条 */
export function IssueFilters({
  searchInput,
  onSearchInput,
  stateFilter,
  onStateFilter,
  labels,
  filterLabel,
  onFilterLabel,
  milestones,
  filterMilestone,
  onFilterMilestone,
}: {
  searchInput: string;
  onSearchInput: (v: string) => void;
  stateFilter: string; // "" | "open" | "closed"
  onStateFilter: (v: string) => void;
  labels: Label[];
  filterLabel: number | null;
  onFilterLabel: (id: number | null) => void;
  milestones: Milestone[];
  filterMilestone: string; // "" | "none" | milestone id
  onFilterMilestone: (v: string) => void;
}) {
  const { t } = useI18n();

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            placeholder={t("issues.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => onSearchInput(e.target.value)}
          />
        </div>
        <div className="flex items-center gap-1">
          {(["", "open", "closed"] as const).map((s) => (
            <Button
              key={s || "all"}
              size="sm"
              variant={stateFilter === s ? "secondary" : "outline"}
              onClick={() => onStateFilter(s)}
            >
              {s === ""
                ? t("issues.filterAll")
                : s === "open"
                  ? t("issues.open")
                  : t("issues.closed")}
            </Button>
          ))}
        </div>
        <select
          aria-label={t("issues.milestone")}
          title={t("issues.milestone")}
          value={filterMilestone}
          onChange={(e) => onFilterMilestone(e.target.value)}
          className={cn(
            "h-9 max-w-44 rounded-md border border-input bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring",
            filterMilestone !== "" && "border-primary text-foreground",
          )}
        >
          <option value="">{t("issues.allMilestones")}</option>
          <option value="none">{t("issues.noMilestone")}</option>
          {milestones.map((m) => (
            <option key={m.id} value={String(m.id)}>
              {m.title}
            </option>
          ))}
        </select>
      </div>

      {labels.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs text-muted-foreground">{t("issues.filterHint")}</span>
          {labels.map((l) => (
            <button
              key={l.id}
              type="button"
              onClick={() => onFilterLabel(filterLabel === l.id ? null : l.id)}
              className={cn(
                "rounded-full outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring",
                filterLabel === l.id ? "ring-2 ring-ring ring-offset-1" : "opacity-70 hover:opacity-100",
              )}
              title={l.name}
            >
              <LabelChip label={l} />
            </button>
          ))}
        </div>
      )}
    </>
  );
}
