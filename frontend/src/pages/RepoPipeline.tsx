import { Fragment, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { CheckCircle2, Loader2, PauseCircle, Play, Terminal, XCircle } from "lucide-react";
import { api, type PipelineRun, type PipelineRunStatus } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
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

export const PIPELINE_EXAMPLE = `image: alpine:3.19
env:
  - CGO_ENABLED=0
steps:
  - name: build
    run: echo build
  - name: test
    run: |
      echo test
      echo done`;

interface Props {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
}

function StatusBadge({ status }: { status: PipelineRunStatus }) {
  const { t } = useI18n();
  const map: Record<PipelineRunStatus, { icon: ReactNode; cls: string }> = {
    pending: { icon: <PauseCircle className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
    running: { icon: <Loader2 className="h-3 w-3 animate-spin" />, cls: "border-blue-600/40 text-blue-600" },
    success: { icon: <CheckCircle2 className="h-3 w-3" />, cls: "border-green-600/40 text-green-600" },
    failed: { icon: <XCircle className="h-3 w-3" />, cls: "border-destructive/50 text-destructive" },
  };
  const s = map[status] ?? map.pending;
  return (
    <Badge variant="outline" className={cn("gap-1", s.cls)}>
      {s.icon}
      {t(`pipeline.status.${status}`)}
    </Badge>
  );
}

export default function RepoPipeline({ owner, name, role }: Props) {
  const { t, to, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const isOwner = role === "owner";
  const canWrite = role === "owner" || role === "write";

  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [runs, setRuns] = useState<PipelineRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [toggling, setToggling] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [runDetail, setRunDetail] = useState<PipelineRun | null>(null);
  const timerRef = useRef<number | null>(null);

  const load = useCallback(async () => {
    try {
      const [cfg, rs] = await Promise.all([api.getPipeline(owner, name), api.listPipelineRuns(owner, name)]);
      setEnabled(cfg.enabled);
      setRuns(rs);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  // 有进行中的运行时轮询刷新
  useEffect(() => {
    const active = runs.some((r) => r.status === "pending" || r.status === "running");
    if (!active) return;
    timerRef.current = window.setTimeout(() => {
      api
        .listPipelineRuns(owner, name)
        .then(setRuns)
        .catch(() => undefined);
    }, 3000);
    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
    };
  }, [runs, owner, name]);

  const toggle = async () => {
    if (enabled === null) return;
    setToggling(true);
    try {
      const res = await api.setPipeline(owner, name, !enabled);
      setEnabled(res.enabled);
      toast.success(t(res.enabled ? "pipeline.enabled" : "pipeline.disabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setToggling(false);
    }
  };

  const trigger = async () => {
    setTriggering(true);
    try {
      const run = await api.triggerPipelineRun(owner, name);
      toast.success(t("pipeline.triggered", { id: run.id }));
      setRuns((rs) => [run, ...rs]);
      setExpanded(run.id);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setTriggering(false);
    }
  };

  const openRun = async (run: PipelineRun) => {
    if (expanded === run.id) {
      setExpanded(null);
      setRunDetail(null);
      return;
    }
    setExpanded(run.id);
    setRunDetail(null);
    try {
      setRunDetail(await api.getPipelineRun(owner, name, run.id));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const refreshOne = async (id: number) => {
    try {
      setRunDetail(await api.getPipelineRun(owner, name, id));
    } catch {
      /* ignore */
    }
  };

  if (loading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-2">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <CardTitle className="text-base">{t("pipeline.title")}</CardTitle>
              <CardDescription className="mt-1">{t("pipeline.hint")}</CardDescription>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {isOwner && (
                <Button
                  variant={enabled ? "default" : "outline"}
                  size="sm"
                  disabled={toggling || enabled === null}
                  onClick={toggle}
                >
                  {enabled ? t("pipeline.turnOff") : t("pipeline.turnOn")}
                </Button>
              )}
              {canWrite && (
                <Button variant="outline" size="sm" className="gap-1.5" disabled={triggering} onClick={trigger}>
                  <Play className="h-3.5 w-3.5" />
                  {t("pipeline.runNow")}
                </Button>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={enabled ? "default" : "secondary"}>
              {enabled === null ? "…" : t(enabled ? "pipeline.statusOn" : "pipeline.statusOff")}
            </Badge>
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">.gitdash.yml</code>
          </div>
        </CardContent>
      </Card>

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
                      <TableRow
                        className="cursor-pointer"
                        onClick={() => openRun(r)}
                      >
                        <TableCell className="font-mono text-xs">{r.id}</TableCell>
                        <TableCell>
                          <StatusBadge status={r.status} />
                          <span className="ml-2 text-xs text-muted-foreground">
                            {r.steps_done}/{r.steps_total}
                          </span>
                        </TableCell>
                        <TableCell>
                          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{r.sha.slice(0, 7)}</code>
                        </TableCell>
                        <TableCell className="truncate text-sm">{r.ref}</TableCell>
                        <TableCell className="truncate text-sm text-muted-foreground">{r.trigger_by}</TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {formatDate(r.created_at, locale)}
                        </TableCell>
                      </TableRow>
                      {expanded === r.id && (
                        <TableRow>
                          <TableCell colSpan={6} className="bg-muted/30 p-0">
                            <div className="p-3">
                              {r.error && (
                                <p className="mb-2 text-xs text-destructive">{r.error}</p>
                              )}
                              {runDetail && runDetail.id === r.id ? (
                                <>
                                  <div className="mb-2 flex items-center justify-between">
                                    <span className="text-xs text-muted-foreground">{t("pipeline.log")}</span>
                                    <div className="flex items-center gap-2">
                                      {(runDetail.status === "pending" || runDetail.status === "running") && (
                                        <Button size="sm" variant="ghost" className="h-7 gap-1 text-xs" onClick={() => refreshOne(r.id)}>
                                          <Loader2 className="h-3 w-3 animate-spin" />
                                          {t("pipeline.refresh")}
                                        </Button>
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

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t("pipeline.dslTitle")}</CardTitle>
          <CardDescription>{t("pipeline.dslHint")}</CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 font-mono text-xs leading-relaxed">
            {PIPELINE_EXAMPLE}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}
