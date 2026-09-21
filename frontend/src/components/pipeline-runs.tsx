import { Fragment } from "react";
import { Loader2, Package, Terminal } from "lucide-react";
import { api, type PipelineRun } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { RelativeTime } from "@/components/relative-time";
import { StatusBadge } from "@/components/pipeline-shared";

interface Props {
  owner: string;
  name: string;
  runs: PipelineRun[];
  expanded: number | null;
  runDetail: PipelineRun | null;
  canWrite: boolean;
  locale: string;
  onOpen: (run: PipelineRun) => void;
  onRerun: (id: number) => void;
  onCancel: (id: number) => void;
  onRefresh: (id: number) => void;
}

/** 运行列表：表格 + 展开后的日志面板与取消/重跑/刷新操作。 */
export default function PipelineRunsCard({
  owner,
  name,
  runs,
  expanded,
  runDetail,
  canWrite,
  locale,
  onOpen,
  onRerun,
  onCancel,
  onRefresh,
}: Props) {
  const { t } = useI18n();
  const finished = (s: PipelineRun["status"]) => s === "success" || s === "failed" || s === "cancelled";
  const active = (s: PipelineRun["status"]) => s === "pending" || s === "running";

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{t("pipeline.runs")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {runs.length === 0 ? (
          <p className="flex items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-sm text-muted-foreground">
            <Terminal className="h-4 w-4" />
            {t("pipeline.noRuns")}
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[640px]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-14">#</TableHead>
                  <TableHead className="w-40">{t("pipeline.fileLabel")}</TableHead>
                  <TableHead className="w-24">{t("pipeline.statusLabel")}</TableHead>
                  <TableHead>{t("repo.commit")}</TableHead>
                  <TableHead className="w-24">{t("pipeline.branch")}</TableHead>
                  <TableHead className="w-32">{t("pipeline.trigger")}</TableHead>
                  <TableHead className="w-44">{t("common.date")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((r) => (
                  <Fragment key={r.id}>
                    <TableRow className="cursor-pointer" onClick={() => onOpen(r)}>
                      <TableCell className="font-mono text-xs">{r.id}</TableCell>
                      <TableCell className="truncate font-mono text-xs" title={r.file || ".gitdash.yml"}>
                        {r.file || ".gitdash.yml"}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={r.status} />
                        <span className="ml-2 text-xs text-muted-foreground">
                          {r.steps_done}/{r.steps_total}
                        </span>
                        {r.status === "pending" && r.run_at && (
                          <span className="ml-2 text-xs text-muted-foreground">
                            {t("pipeline.scheduledFor", { time: formatDate(r.run_at, locale) })}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{r.sha.slice(0, 7)}</code>
                      </TableCell>
                      <TableCell className="truncate text-sm">{r.ref}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        <div className="flex items-center gap-1.5">
                          {r.event && (
                            <Badge variant="secondary" className="shrink-0 text-[10px]">
                              {t(`pipeline.event.${r.event}`)}
                            </Badge>
                          )}
                          <span className="truncate">{r.trigger_by}</span>
                        </div>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        <RelativeTime iso={r.created_at} locale={locale} />
                      </TableCell>
                    </TableRow>
                    {expanded === r.id && (
                      <TableRow>
                        <TableCell colSpan={7} className="bg-muted/30 p-0">
                          <div className="p-3">
                            {r.error && <p className="mb-2 text-xs text-destructive">{r.error}</p>}
                            {runDetail && runDetail.id === r.id ? (
                              <>
                                <div className="mb-2 flex items-center justify-between">
                                  <span className="text-xs text-muted-foreground">
                                    {t("pipeline.log")}
                                    {runDetail.runner_name && (
                                      <span className="ml-2 rounded bg-muted px-1.5 py-0.5 font-mono">
                                        runner: {runDetail.runner_name}
                                      </span>
                                    )}
                                  </span>
                                  <div className="flex items-center gap-2">
                                    {runDetail.has_artifacts && (
                                      <a
                                        className="inline-flex h-7 items-center gap-1 rounded-md px-2 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
                                        href={api.runArtifactsUrl(owner, name, r.id)}
                                        download
                                      >
                                        <Package className="h-3 w-3" />
                                        {t("pipeline.artifacts")}
                                      </a>
                                    )}
                                    {canWrite && finished(runDetail.status) && (
                                      <Button
                                        size="sm"
                                        variant="ghost"
                                        className="h-7 gap-1 text-xs"
                                        onClick={() => onRerun(r.id)}
                                      >
                                        {t("pipeline.rerun")}
                                      </Button>
                                    )}
                                    {active(runDetail.status) && (
                                      <>
                                        {canWrite && (
                                          <Button
                                            size="sm"
                                            variant="ghost"
                                            className="h-7 gap-1 text-xs"
                                            onClick={() => onCancel(r.id)}
                                          >
                                            {t("pipeline.cancel")}
                                          </Button>
                                        )}
                                        <Button
                                          size="sm"
                                          variant="ghost"
                                          className="h-7 gap-1 text-xs"
                                          onClick={() => onRefresh(r.id)}
                                        >
                                          <Loader2 className="h-3 w-3 animate-spin" />
                                          {t("pipeline.refresh")}
                                        </Button>
                                      </>
                                    )}
                                  </div>
                                </div>
                                <pre className="max-h-[50vh] overflow-auto rounded-md border bg-background/80 p-3 font-mono text-xs leading-relaxed">
                                  {runDetail.log || "…"}
                                </pre>
                              </>
                            ) : (
                              <p className="py-6 text-center text-sm text-muted-foreground">…</p>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
