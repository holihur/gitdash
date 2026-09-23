import { Link } from "react-router-dom";
import { CheckCircle2, Circle, Flag, MessageSquare, Pin, UserRound } from "lucide-react";
import type { Issue } from "@/lib/api";
import { cn, formatDate } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { buildIssuePath } from "@/lib/repo-url";
import LabelChip from "@/components/label-chip";
import { IssueActions } from "@/components/issues/issue-actions";

/** issue 列表行：标题链接到独立的详情页，行尾保留快捷操作。 */
export function IssueItem({
  issue,
  owner,
  name,
  canWrite,
  onChanged,
}: {
  issue: Issue;
  owner: string;
  name: string;
  canWrite: boolean;
  onChanged: () => void;
}) {
  const { t, lang } = useI18n();
  const locale = dateLocale(lang);
  const isOpen = issue.state === "open";
  const issueLabels = issue.labels ?? [];

  return (
    <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:gap-3">
      <Link
        to={buildIssuePath(owner, name, issue.number)}
        className="flex min-w-0 flex-1 items-start gap-2 text-left"
      >
        {isOpen ? (
          <Circle className="mt-0.5 h-4 w-4 shrink-0 fill-green-500 text-green-600" />
        ) : (
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        )}
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-1">
            {issue.pinned && <Pin className="h-3.5 w-3.5 shrink-0 fill-current text-amber-500" />}
            <span
              className={cn(
                "min-w-0 truncate font-medium hover:underline",
                !isOpen && "text-muted-foreground",
              )}
            >
              {issue.title}
            </span>
          </span>
          {(issueLabels.length > 0 || issue.milestone || (issue.assignees?.length ?? 0) > 0) && (
            <span className="mt-1 flex flex-wrap items-center gap-1">
              {issueLabels.map((l) => (
                <LabelChip key={l.id} label={l} />
              ))}
              {issue.milestone && (
                <span
                  className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs text-muted-foreground"
                  title={issue.milestone.description || issue.milestone.title}
                >
                  <Flag className="h-3 w-3" />
                  <span className="max-w-40 truncate">{issue.milestone.title}</span>
                </span>
              )}
              {(issue.assignees ?? []).map((u) => (
                <span
                  key={u}
                  className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs text-muted-foreground"
                  title={u}
                >
                  <UserRound className="h-3 w-3" />
                  <span className="max-w-40 truncate">{u}</span>
                </span>
              ))}
            </span>
          )}
          <span className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
            <span className="truncate">
              #{issue.number} ·{" "}
              {isOpen
                ? t("issues.openedOn", {
                    author: issue.author,
                    date: formatDate(issue.created_at, locale),
                  })
                : t("issues.closedOn", {
                    author: issue.author,
                    date: formatDate(issue.closed_at ?? issue.updated_at, locale),
                  })}
            </span>
            {(issue.comment_count ?? 0) > 0 && (
              <span className="inline-flex shrink-0 items-center gap-0.5">
                <MessageSquare className="h-3 w-3" />
                {issue.comment_count}
              </span>
            )}
          </span>
        </span>
      </Link>
      <div className="pl-6 sm:pl-0">
        <IssueActions
          owner={owner}
          name={name}
          issue={issue}
          canWrite={canWrite}
          onChanged={onChanged}
        />
      </div>
    </div>
  );
}
