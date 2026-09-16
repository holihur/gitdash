import { ChevronDown, ChevronRight, GitBranch, GitBranchPlus, GitCompare, Tag as TagIcon } from "lucide-react";
import type { Branch, Tag } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { CompareDialog } from "@/components/compare-dialog";

interface Props {
  owner: string;
  name: string;
  refName: string;
  branches: Branch[];
  tags: Tag[];
  emptyRepo: boolean;
  crumbs: string[];
  hasBlob: boolean;
  compareOpen: boolean;
  onCompareOpenChange: (v: boolean) => void;
  onSelectRef: (ref: string) => void;
  onOpenRefs: () => void;
  onOpenDir: (dir: string) => void;
}

/** 代码浏览的 ref 选择条：分支/tag 下拉、refs 管理、比较与面包屑。 */
export default function CodeRefBar({
  owner,
  name,
  refName,
  branches,
  tags,
  emptyRepo,
  crumbs,
  hasBlob,
  compareOpen,
  onCompareOpenChange,
  onSelectRef,
  onOpenRefs,
  onOpenDir,
}: Props) {
  const { t } = useI18n();
  return (
    <div className="flex flex-wrap items-center gap-2">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" className="max-w-full gap-2" disabled={emptyRepo}>
            <GitBranch className="h-4 w-4 shrink-0" />
            <span className="truncate">{refName || t("repo.noBranch")}</span>
            <ChevronDown className="h-3.5 w-3.5 shrink-0" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="max-h-72 w-64 overflow-auto">
          {branches.map((b) => (
            <DropdownMenuItem key={b.name} onClick={() => onSelectRef(b.name)}>
              <GitBranch className="shrink-0" />
              <span className="truncate">{b.name}</span>
              {b.is_head && (
                <Badge variant="secondary" className="ml-auto shrink-0">
                  HEAD
                </Badge>
              )}
            </DropdownMenuItem>
          ))}
          {tags.length > 0 && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuLabel className="text-xs">{t("refs.tags")}</DropdownMenuLabel>
              {tags.map((tg) => (
                <DropdownMenuItem key={tg.name} onClick={() => onSelectRef(tg.name)}>
                  <TagIcon className="shrink-0" />
                  <span className="truncate">{tg.name}</span>
                </DropdownMenuItem>
              ))}
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <Button variant="ghost" size="sm" className="h-9 w-9 px-0" title={t("refs.manage")} onClick={onOpenRefs}>
        <GitBranchPlus className="h-4 w-4" />
      </Button>

      <Button
        variant="outline"
        size="sm"
        className="gap-1.5"
        disabled={emptyRepo}
        onClick={() => onCompareOpenChange(true)}
      >
        <GitCompare className="h-4 w-4" />
        {t("compare.title")}
      </Button>

      <CompareDialog
        owner={owner}
        name={name}
        branches={branches}
        tags={tags}
        defaultRef={refName}
        open={compareOpen}
        onOpenChange={onCompareOpenChange}
      />

      {!emptyRepo && (
        <nav className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
          <button className="font-medium hover:underline" onClick={() => onOpenDir("")}>
            {name}
          </button>
          {crumbs.map((seg, i) => {
            const isFile = hasBlob && i === crumbs.length - 1;
            return (
              <span key={`${seg}-${i}`} className="flex items-center gap-1">
                <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                {isFile ? (
                  <span className="font-medium">{seg}</span>
                ) : (
                  <button
                    className={cn(
                      "hover:underline",
                      i === crumbs.length - 1 ? "font-medium" : "text-muted-foreground",
                    )}
                    onClick={() => onOpenDir(crumbs.slice(0, i + 1).join("/"))}
                  >
                    {seg}
                  </button>
                )}
              </span>
            );
          })}
        </nav>
      )}
    </div>
  );
}
