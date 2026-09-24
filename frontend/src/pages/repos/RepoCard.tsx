import { Link } from "react-router-dom";
import { Eye, MoreVertical, Star, Trash2, Users, Webhook } from "lucide-react";
import type { Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useI18n } from "@/lib/i18n";
import { RelativeTime } from "@/components/relative-time";
import { LanguageBadge } from "@/components/language-bar";
import { BadgeStrip } from "@/components/badge-strip";

interface Props {
  repo: Repo;
  isMine: boolean;
  isOwner: boolean;
  locale: string;
  onManageCollabs: (repo: Repo) => void;
  onManageWebhooks: (repo: Repo) => void;
  onDelete: (repo: Repo) => void;
}

/** 仓库列表卡片：名称、描述、统计与 owner 操作下拉。 */
export default function RepoCard({
  repo,
  isMine,
  isOwner,
  locale,
  onManageCollabs,
  onManageWebhooks,
  onDelete,
}: Props) {
  const { t } = useI18n();
  return (
    <Card className="flex min-w-0 flex-col">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="min-w-0 text-lg">
            <Link to={`/repo/${repo.owner}/${repo.name}`} className="block truncate hover:underline">
              {repo.name}
            </Link>
          </CardTitle>
          {isOwner && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 shrink-0"
                  aria-label={`${repo.name} ${t("common.moreActions")}`}
                >
                  <MoreVertical className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => onManageCollabs(repo)}>
                  <Users />
                  {t("collabs.manage")}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => onManageWebhooks(repo)}>
                  <Webhook />
                  {t("webhooks.manage")}
                </DropdownMenuItem>
                <DropdownMenuItem
                  className="text-destructive focus:text-destructive"
                  onClick={() => onDelete(repo)}
                >
                  <Trash2 />
                  {t("repos.delete")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
        <CardDescription className="min-h-10">
          <span className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
            {!isMine && (
              <Badge variant="secondary" className="font-normal">
                {repo.owner}
              </Badge>
            )}
            {isMine && !isOwner && repo.role && (
              <Badge variant="secondary" className="font-normal">
                {t("collabs.sharedBy", { owner: repo.owner })} ·
                {repo.role === "write" ? t("collabs.write") : t("collabs.read")}
              </Badge>
            )}
          </span>
          <span className="line-clamp-2 block">
            {repo.description || t("common.noDescription")}
          </span>
          <BadgeStrip kind="repo" owner={repo.owner} repo={repo.name} className="mt-1" />
        </CardDescription>
      </CardHeader>
      <CardContent className="mt-auto space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary" className="font-normal">
            <RelativeTime iso={repo.created_at} locale={locale} />
          </Badge>
          {repo.language && (
            <Badge variant="secondary" className="font-normal">
              <LanguageBadge language={repo.language} />
            </Badge>
          )}
          {repo.is_template && (
            <Badge variant="outline" className="font-normal">
              {t("repos.templateBadge")}
            </Badge>
          )}
          <Badge variant="secondary" className="gap-1 font-normal">
            <Star className="h-3 w-3" />
            {repo.stars ?? 0}
          </Badge>
          <Badge variant="secondary" className="gap-1 font-normal">
            <Eye className="h-3 w-3" />
            {repo.watchers ?? 0}
          </Badge>
          {repo.import_status && repo.import_status !== "synced" && (
            <Badge
              variant="outline"
              className={
                repo.import_status === "failed"
                  ? "font-normal text-destructive"
                  : "font-normal text-amber-600 dark:text-amber-400"
              }
              title={repo.import_error || undefined}
            >
              {t(`imports.status.${repo.import_status}`)}
            </Badge>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
