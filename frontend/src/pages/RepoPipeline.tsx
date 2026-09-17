import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Loader2, Play, Workflow } from "lucide-react";
import { api, type PipelineGraph, type PipelineRun } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { MermaidDiagram } from "@/components/mermaid";
import PipelineDocs from "@/components/pipeline-docs";
import PipelineRunsCard from "@/components/pipeline-runs";
import { toMermaid } from "@/components/pipeline-shared";

interface Props {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
}

export default function RepoPipeline({ owner, name, role }: Props) {
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
  const isOwner = role === "owner";
  const canWrite = role === "owner" || role === "write";

  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [files, setFiles] = useState<string[]>([]);
  const [file, setFile] = useState("");
  const [runs, setRuns] = useState<PipelineRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [toggling, setToggling] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [ref, setRef] = useState("");
  const [delay, setDelay] = useState("");
  const [expanded, setExpanded] = useState<number | null>(null);
  const [runDetail, setRunDetail] = useState<PipelineRun | null>(null);
  const [graph, setGraph] = useState<PipelineGraph | null>(null);
  const [graphBusy, setGraphBusy] = useState(false);
  const [graphErr, setGraphErr] = useState("");
  const timerRef = useRef<number | null>(null);

  const load = useCallback(async () => {
    try {
      const [cfg, rs] = await Promise.all([api.getPipeline(owner, name), api.listPipelineRuns(owner, name)]);
      setEnabled(cfg.enabled);
      const discovered = cfg.files ?? [];
      setFiles(discovered);
      setFile((prev) => prev || discovered[0] || "");
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

  const loadGraph = async () => {
    setGraphBusy(true);
    setGraphErr("");
    try {
      setGraph(await api.getPipelineGraph(owner, name, ref.trim() || undefined, file || undefined));
    } catch (e) {
      setGraphErr(apiErrorMsg(to, e));
      setGraph(null);
    } finally {
      setGraphBusy(false);
    }
  };

  const trigger = async () => {
    setTriggering(true);
    try {
      const target = ref.trim();
      const body: { ref?: string; file?: string; delay?: string } = {};
      if (target) body.ref = target;
      if (file) body.file = file;
      const delayVal = delay.trim();
      if (delayVal) body.delay = delayVal;
      const { runs: created } = await api.triggerPipelineRun(owner, name, body);
      if (created.length === 0) return;
      toast.success(t("pipeline.triggered", { id: created.map((r) => r.id).join(", ") }));
      setRuns((rs) => [...created, ...rs]);
      setExpanded(created[0].id);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setTriggering(false);
    }
  };

  const rerun = async (id: number) => {
    try {
      const run = await api.rerunPipelineRun(owner, name, id);
      toast.success(t("pipeline.triggered", { id: run.id }));
      setRuns((rs) => [run, ...rs]);
      setExpanded(run.id);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
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

  const cancelRun = async (id: number) => {
    try {
      await api.cancelPipelineRun(owner, name, id);
      toast.success(t("pipeline.cancelled"));
      refreshOne(id);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
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
                <div className="flex items-center gap-2">
                  {files.length > 0 && (
                    <select
                      value={file}
                      onChange={(e) => setFile(e.target.value)}
                      className="h-8 max-w-[14rem] rounded-md border border-input bg-background px-2 text-xs"
                      title={t("pipeline.fileLabel")}
                    >
                      {files.length > 1 && <option value="">{t("pipeline.allFiles")}</option>}
                      {files.map((f) => (
                        <option key={f} value={f}>{f}</option>
                      ))}
                    </select>
                  )}
                  <Input
                    value={ref}
                    onChange={(e) => setRef(e.target.value)}
                    placeholder={t("pipeline.refPlaceholder")}
                    className="h-8 w-40 text-xs"
                  />
                  <Input
                    value={delay}
                    onChange={(e) => setDelay(e.target.value)}
                    placeholder={t("pipeline.delayPlaceholder")}
                    title={t("pipeline.delayHint")}
                    className="h-8 w-28 text-xs"
                  />
                  <Button variant="outline" size="sm" className="gap-1.5" disabled={triggering} onClick={trigger}>
                    <Play className="h-3.5 w-3.5" />
                    {t("pipeline.runNow")}
                  </Button>
                </div>
              )}
              <Button
                variant="outline"
                size="sm"
                className="gap-1.5"
                disabled={graphBusy}
                onClick={loadGraph}
              >
                {graphBusy ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Workflow className="h-3.5 w-3.5" />
                )}
                {t("pipeline.visualize")}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={enabled ? "default" : "secondary"}>
              {enabled === null ? "…" : t(enabled ? "pipeline.statusOn" : "pipeline.statusOff")}
            </Badge>
            {(files.length > 0 ? files : [".gitdash.yml"]).map((f) => (
              <code key={f} className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{f}</code>
            ))}
          </div>
        </CardContent>
      </Card>

      {(graph || graphErr || graphBusy) && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">{t("pipeline.visualize")}</CardTitle>
            <CardDescription>
              {graph ? `${graph.file ?? ".gitdash.yml"} · ${graph.ref}${graph.image ? " · " + graph.image : ""}` : t("pipeline.hint")}
            </CardDescription>
          </CardHeader>
          <CardContent className="overflow-x-auto">
            {graphBusy ? (
              <p className="py-6 text-center text-sm text-muted-foreground">…</p>
            ) : graphErr ? (
              <p className="text-sm text-destructive">{graphErr}</p>
            ) : graph ? (
              <MermaidDiagram chart={toMermaid(graph)} />
            ) : null}
          </CardContent>
        </Card>
      )}

      <PipelineRunsCard
        owner={owner}
        name={name}
        runs={runs}
        expanded={expanded}
        runDetail={runDetail}
        canWrite={canWrite}
        locale={locale}
        onOpen={openRun}
        onRerun={rerun}
        onCancel={cancelRun}
        onRefresh={refreshOne}
      />

      <PipelineDocs />
    </div>
  );
}

