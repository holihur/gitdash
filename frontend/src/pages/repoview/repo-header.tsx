import { Link, useNavigate } from "react-router-dom";
import { Copy, Eye, GitBranch, GitFork, HardDrive, MoreVertical, Star } from "lucide-react";
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
import { cn, formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { cloneCommand, type Repo } from "@/lib/api";
import { BadgeStrip } from "@/components/badge-strip";

interface Props {
  owner: string;
  name: string;
  repo: Repo | null;
  isOwner: boolean;
  watchBusy: boolean;
  starBusy: boolean;
  onToggleWatch: () => void;
  onToggleStar: () => void;
  onFork: () => void;
  onCopy: (text: string) => void;
}

/** 仓库页头部：名称/描述/topics/fork 来源 + watch/star/fork/克隆操作。 */
export default function RepoHeader({
  owner,
  name,
  repo,
  isOwner,
  watchBusy,
  starBusy,
  onToggleWatch,
  onToggleStar,
  onFork,
  onCopy,
}: Props) {
  const { t } = useI18n();
  const navigate = useNavigate();
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <div className="min-w-0">
        <h1 className="truncate text-2xl font-bold">
          <Link to={`/users/${owner}`} className="hover:underline">
            {owner}
          </Link>
          <span className="text-muted-foreground">/</span>
          {name}
        </h1>
        <BadgeStrip kind="repo" owner={owner} repo={name} className="mt-1" />
        <p className="text-sm text-muted-foreground">{repo?.description || t("common.noDescription")}</p>
        {repo?.topics && repo.topics.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {repo.topics.map((tp) => (
              <Link key={tp} to={`/explore?tag=${encodeURIComponent(tp)}`}>
                <Badge variant="secondary" className="font-normal">
                  {tp}
                </Badge>
              </Link>
            ))}
          </div>
        )}
        {repo?.fork_owner && repo?.fork_repo && (
          <p className="text-xs text-muted-foreground">
            {t("social.forkedFrom")}{" "}
            <button
              className="hover:underline"
              onClick={() => navigate(`/repo/${repo.fork_owner}/${repo.fork_repo}`)}
            >
              {repo.fork_owner}/{repo.fork_repo}
            </button>
          </p>
        )}
      </div>
      <div className="flex flex-nowrap items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          className="h-8 shrink-0 gap-1 px-2 text-xs [&_svg]:size-3.5 sm:h-9 sm:gap-1.5 sm:px-3 sm:text-sm sm:[&_svg]:size-4"
          disabled={watchBusy}
          onClick={onToggleWatch}
          title={t("social.watchTitle")}
        >
          <Eye className={cn("h-4 w-4", repo?.watching && "fill-current text-blue-500")} />
          <span className="hidden sm:inline">{repo?.watching ? t("social.watchingBtn") : t("social.watch")}</span>
          <span className="text-muted-foreground">{repo?.watchers ?? 0}</span>
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-8 shrink-0 gap-1 px-2 text-xs [&_svg]:size-3.5 sm:h-9 sm:gap-1.5 sm:px-3 sm:text-sm sm:[&_svg]:size-4"
          disabled={starBusy}
          onClick={onToggleStar}
        >
          <Star className={cn("h-4 w-4", repo?.starred && "fill-current text-yellow-500")} />
          <span className="hidden sm:inline">{repo?.starred ? t("social.starredBtn") : t("social.star")}</span>
          <span className="text-muted-foreground">{repo?.stars ?? 0}</span>
        </Button>
        {typeof repo?.size === "number" && (
          <span
            className="inline-flex h-8 shrink-0 items-center gap-1 rounded-md border border-input bg-background px-2 text-xs text-muted-foreground [&_svg]:size-3.5 sm:h-9 sm:gap-1.5 sm:px-3 sm:text-sm sm:[&_svg]:size-4"
            title={t("common.size")}
          >
            <HardDrive className="h-4 w-4" />
            {formatSize(repo.size)}
          </span>
        )}
        {!isOwner && (
          <Button
            variant="outline"
            size="sm"
            className="h-8 shrink-0 gap-1 px-2 text-xs [&_svg]:size-3.5 sm:h-9 sm:gap-1.5 sm:px-3 sm:text-sm sm:[&_svg]:size-4"
            onClick={onFork}
          >
            <GitFork className="h-4 w-4" />
            <span className="hidden sm:inline">{t("social.fork")}</span>
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              className="h-8 shrink-0 gap-1.5 px-2 font-mono text-xs [&_svg]:size-3.5 sm:h-9 sm:gap-2 sm:px-3 sm:[&_svg]:size-4"
            >
              <GitBranch className="h-3.5 w-3.5" />
              SSH
              <MoreVertical className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-80 max-w-[calc(100vw-2rem)]">
            <DropdownMenuLabel className="break-all font-mono text-xs normal-case">
              {cloneCommand(owner, name)}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            {/* onSelect 而非 onClick：移动端菜单项在 pointerup 后关闭，
                用 onSelect 能确保选中时同步触发复制（保留用户手势）。 */}
            <DropdownMenuItem onSelect={() => onCopy(cloneCommand(owner, name))}>
              <Copy />
              {t("repo.copyCloneCommand")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
