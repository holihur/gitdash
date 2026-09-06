import { useEffect, useState } from "react";
import { toast } from "sonner";
import { BadgeCheck, ChevronDown, GitCommitHorizontal } from "lucide-react";
import { api, type Commit, type PullDiff } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn, formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { DiffView, type DiffFileInfo } from "@/components/diff-view";

export interface CommitsTabProps {
  owner: string;
  name: string;
  refName: string;
  emptyRepo: boolean;
}

export default function CommitsTab({ owner, name, refName, emptyRepo }: CommitsTabProps) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [commits, setCommits] = useState<Commit[]>([]);
  const [diffSha, setDiffSha] = useState<string | null>(null);
  const [diffData, setDiffData] = useState<{ files: DiffFileInfo[]; patch: string } | null>(null);

  useEffect(() => {
    if (!refName) return;
    api
      .commits(owner, name, refName)
      .then(setCommits)
      .catch((e) => toast.error(apiErrorMsg(to, e)));
  }, [owner, name, refName, to]);

  useEffect(() => {
    if (!diffSha) {
      setDiffData(null);
      return;
    }
    let alive = true;
    api
      .commitDiff(owner, name, diffSha)
      .then((d: PullDiff) => alive && setDiffData({ files: d.files, patch: d.patch }))
      .catch(() => alive && setDiffData(null));
    return () => {
      alive = false;
    };
  }, [diffSha, owner, name]);

  return (
    <div>
      {emptyRepo ? (
        <p className="py-10 text-center text-sm text-muted-foreground">{t("repo.noCommits")}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[640px]">
            <TableHeader>
              <TableRow>
                <TableHead>{t("repo.commit")}</TableHead>
                <TableHead className="w-36">{t("common.author")}</TableHead>
                <TableHead className="w-56">{t("common.date")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {commits.map((c) => (
                <TableRow key={c.sha}>
                  <TableCell>
                    <button
                      type="button"
                      className="flex w-full items-center gap-3 text-left"
                      onClick={() => setDiffSha(diffSha === c.sha ? null : c.sha)}
                      title={t("commits.viewDiff")}
                    >
                      <GitCommitHorizontal className="h-4 w-4 shrink-0 text-muted-foreground" />
                      <code className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs">
                        {c.sha.slice(0, 7)}
                      </code>
                      <span className="truncate">{c.message}</span>
                      {c.gpg_verified && (
                        <Badge
                          variant="outline"
                          className="shrink-0 gap-1 border-green-600/40 text-green-600"
                          title={t("commits.gpgSigned", { user: c.gpg_verified })}
                        >
                          <BadgeCheck className="h-3 w-3" />
                          {c.gpg_verified}
                        </Badge>
                      )}
                      <ChevronDown
                        className={cn(
                          "h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform",
                          diffSha === c.sha && "rotate-180",
                        )}
                      />
                      </button>
                  </TableCell>
                  <TableCell className="text-sm">{c.author}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(c.date, locale)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {diffSha && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="font-mono text-sm">{diffSha.slice(0, 12)}</CardTitle>
          </CardHeader>
          <CardContent>
            {diffData ? (
              <DiffView files={diffData.files} patch={diffData.patch} />
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">…</p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
