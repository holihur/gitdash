import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import {
  BadgeCheck,
  ChevronDown,
  GitBranch,
  GitCommitHorizontal,
  Search,
  ShieldAlert,
  ShieldQuestion,
  Tag,
  Undo2,
} from "lucide-react";
import { api, type Commit, type PullDiff } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import ConfirmDialog from "@/components/confirm-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { RelativeTime } from "@/components/relative-time";
import { apiErrorMsg } from "@/lib/errors";
import { DiffView, type DiffFileInfo } from "@/components/diff-view";
import { buildCommitGraph, laneColor, type CommitGraphRow } from "@/lib/commit-graph";

export interface CommitsTabProps {
  owner: string;
  name: string;
  refName: string;
  emptyRepo: boolean;
  role?: string;
}

// 提交图几何参数（与表格行高保持一致，确保连线对齐）。
const GRAPH_COL_W = 14;
const GRAPH_PAD = 10;
const GRAPH_ROW_H = 44;
// 每次加载的提交条数（与后端默认值一致）。
const PAGE_SIZE = 30;

function CommitGraphCell({ row }: { row: CommitGraphRow }) {
  const x = (lane: number) => GRAPH_PAD + lane * GRAPH_COL_W;
  const mid = GRAPH_ROW_H / 2;
  const q = (GRAPH_ROW_H - mid) / 2;
  const width = row.lanes * GRAPH_COL_W + GRAPH_PAD * 2;
  const parents = row.parentLanes;
  return (
    <svg width={width} height={GRAPH_ROW_H} className="block" aria-hidden="true">
      {row.through.map((i) => (
        <line
          key={`t${i}`}
          x1={x(i)}
          y1={0}
          x2={x(i)}
          y2={GRAPH_ROW_H}
          stroke={laneColor(i)}
          strokeWidth={2}
        />
      ))}
      {row.mergesIn.map((i) => (
        <path
          key={`m${i}`}
          d={`M ${x(i)} 0 Q ${x(i)} ${mid} ${x(row.lane)} ${mid}`}
          fill="none"
          stroke={laneColor(i)}
          strokeWidth={2}
        />
      ))}
      {parents.map((pl, k) =>
        pl === row.lane ? (
          <line
            key={`p${k}`}
            x1={x(row.lane)}
            y1={mid}
            x2={x(row.lane)}
            y2={GRAPH_ROW_H}
            stroke={laneColor(row.lane)}
            strokeWidth={2}
          />
        ) : (
          <path
            key={`p${k}`}
            d={`M ${x(row.lane)} ${mid} C ${x(row.lane)} ${mid + q}, ${x(pl)} ${
              GRAPH_ROW_H - q
            }, ${x(pl)} ${GRAPH_ROW_H}`}
            fill="none"
            stroke={laneColor(pl)}
            strokeWidth={2}
          />
        ),
      )}
      {row.fromTop && (
        <line
          x1={x(row.lane)}
          y1={0}
          x2={x(row.lane)}
          y2={mid}
          stroke={laneColor(row.lane)}
          strokeWidth={2}
        />
      )}
      <circle cx={x(row.lane)} cy={mid} r={4.5} fill={laneColor(row.lane)} />
    </svg>
  );
}

/** 渲染一条 ref 装饰（HEAD -> main / main / tag: v1）。 */
function RefBadge({ label }: { label: string }) {
  const isTag = label.startsWith("tag: ");
  const isHead = label.startsWith("HEAD -> ");
  const text = isTag ? label.slice(5) : isHead ? label.slice(8) : label;
  return (
    <Badge
      variant="outline"
      className={cn(
        "shrink-0 gap-1 font-normal",
        isHead && "border-primary/50 text-primary",
        isTag && "border-amber-500/50 text-amber-600 dark:text-amber-400",
      )}
      title={isHead ? `HEAD → ${text}` : label}
    >
      {isTag ? <Tag className="h-3 w-3" /> : <GitBranch className="h-3 w-3" />}
      {text}
    </Badge>
  );
}

export default function CommitsTab({ owner, name, refName, emptyRepo, role }: CommitsTabProps) {
  const { t, lang, to } = useI18n();
  const locale = dateLocale(lang);
  const [commits, setCommits] = useState<Commit[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const reqId = useRef(0);
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [diffSha, setDiffSha] = useState<string | null>(null);
  const [diffData, setDiffData] = useState<{ files: DiffFileInfo[]; patch: string } | null>(null);
  const [revertSha, setRevertSha] = useState<string | null>(null);
  const [revertBusy, setRevertBusy] = useState(false);
  const [reload, setReload] = useState(0);
  const canWrite = role === "owner" || role === "write";

  // 搜索输入防抖，避免每次按键都请求后端。
  useEffect(() => {
    const id = window.setTimeout(() => setDebouncedQuery(query.trim()), 300);
    return () => window.clearTimeout(id);
  }, [query]);

  // 切换分支 / 搜索词 / 撤销后重置为第一页。
  useEffect(() => {
    if (!refName) return;
    const id = ++reqId.current;
    setLoadingMore(false);
    api
      .commits(owner, name, refName, debouncedQuery || undefined, PAGE_SIZE, 0)
      .then((cs) => {
        if (id !== reqId.current) return;
        setCommits(cs);
        setHasMore(cs.length === PAGE_SIZE);
      })
      .catch((e) => {
        if (id !== reqId.current) return;
        toast.error(apiErrorMsg(to, e));
      });
  }, [owner, name, refName, debouncedQuery, to, reload]);

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

  const graphRows = useMemo(() => buildCommitGraph(commits), [commits]);

  const doRevert = async () => {
    if (!revertSha) return;
    setRevertBusy(true);
    try {
      const r = await api.revertCommit(owner, name, revertSha, refName);
      toast.success(t("commits.reverted", { sha: r.sha.slice(0, 7) }));
      setRevertSha(null);
      setReload((n) => n + 1);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setRevertBusy(false);
    }
  };

  const loadMore = async () => {
    if (loadingMore || !hasMore) return;
    setLoadingMore(true);
    const id = reqId.current;
    try {
      const cs = await api.commits(
        owner,
        name,
        refName,
        debouncedQuery || undefined,
        PAGE_SIZE,
        commits.length,
      );
      if (id !== reqId.current) return;
      setCommits((prev) => [...prev, ...cs]);
      setHasMore(cs.length === PAGE_SIZE);
    } catch (e) {
      if (id === reqId.current) toast.error(apiErrorMsg(to, e));
    } finally {
      if (id === reqId.current) setLoadingMore(false);
    }
  };

  const searching = debouncedQuery !== "";

  return (
    <div>
      {!emptyRepo && (
        <div className="mb-3 flex items-center justify-between gap-3">
          <div className="relative w-full max-w-xs">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("commits.searchPlaceholder")}
              className="pl-8"
              aria-label={t("commits.searchPlaceholder")}
            />
          </div>
        </div>
      )}

      {emptyRepo ? (
        <p className="py-10 text-center text-sm text-muted-foreground">{t("repo.noCommits")}</p>
      ) : commits.length === 0 ? (
        <p className="py-10 text-center text-sm text-muted-foreground">
          {searching ? t("commits.noMatches") : t("repo.noCommits")}
        </p>
      ) : (
        <>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[720px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-16" aria-label={t("commits.graph")} />
                <TableHead>{t("repo.commit")}</TableHead>
                <TableHead className="w-36">{t("common.author")}</TableHead>
                <TableHead className="w-56">{t("common.date")}</TableHead>
                {canWrite && <TableHead className="w-28 text-right">{t("common.actions")}</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {commits.map((c, i) => {
                const row = graphRows[i] ?? {
                  lane: 0,
                  lanes: 1,
                  through: [],
                  mergesIn: [],
                  parentLanes: [],
                  fromTop: false,
                };
                return (
                  <TableRow key={c.sha}>
                    <TableCell className="w-16 p-0 align-middle">
                      <CommitGraphCell row={row} />
                    </TableCell>
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
                        {(c.refs ?? []).map((r) => (
                          <RefBadge key={r} label={r} />
                        ))}
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
                        {!c.gpg_verified && c.gpg_status === "unknown_key" && (
                          <Badge
                            variant="outline"
                            className="shrink-0 text-amber-600 dark:text-amber-400"
                            title={t("commits.gpgUnknownKey")}
                          >
                            <ShieldQuestion className="h-3 w-3" />
                            {t("commits.gpgUnknownKeyShort")}
                          </Badge>
                        )}
                        {!c.gpg_verified && c.gpg_status === "invalid" && (
                          <Badge
                            variant="outline"
                            className="shrink-0 gap-1 border-destructive/40 text-destructive"
                            title={t("commits.gpgInvalid")}
                          >
                            <ShieldAlert className="h-3 w-3" />
                            {t("commits.gpgInvalidShort")}
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
                      <RelativeTime iso={c.date} locale={locale} />
                    </TableCell>
                    {canWrite && (
                      <TableCell className="text-right">
                        <Button
                          variant="outline"
                          size="sm"
                          className="gap-1.5"
                          title={t("commits.revertTitle")}
                          onClick={() => setRevertSha(c.sha)}
                        >
                          <Undo2 className="h-3.5 w-3.5" />
                          {t("commits.revert")}
                        </Button>
                      </TableCell>
                    )}
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
        {hasMore && (
          <div className="mt-3 flex justify-center">
            <Button variant="outline" size="sm" onClick={loadMore} disabled={loadingMore}>
              {loadingMore ? t("commits.loading") : t("commits.loadMore")}
            </Button>
          </div>
        )}
        </>
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
      <ConfirmDialog
        open={revertSha !== null}
        onOpenChange={(o) => !o && setRevertSha(null)}
        title={t("commits.revertTitle")}
        description={t("commits.revertConfirm", {
          sha: (revertSha ?? "").slice(0, 7),
          branch: refName,
        })}
        confirmText={t("commits.revert")}
        onConfirm={doRevert}
        busy={revertBusy}
      />
    </div>
  );
}
