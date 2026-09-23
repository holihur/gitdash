import { useCallback, useEffect, useState, type ReactNode } from "react";
import { Search } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { adminReq, ApiDisabledError } from "../api";

type CodeSearchMetrics = {
  backend: string;
  remote?: boolean;
  search_requests?: { index?: number; grep?: number; indexing?: number };
  index_runs?: { full?: number; incremental?: number; ok?: number; error?: number };
  index?: { repos?: number; documents?: number; dirty?: number };
  index_avg_duration_ms?: number;
  last_index_at?: number;
  index_error?: string;
};

function Stat({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-md border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-lg font-semibold tabular-nums" title={String(value)}>
        {value}
      </div>
    </div>
  );
}

export function CodeSearchSection() {
  const { t } = useI18n();
  const [m, setM] = useState<CodeSearchMetrics | null>(null);

  const load = useCallback(async () => {
    try {
      setM(await adminReq<CodeSearchMetrics>("/codesearch"));
    } catch (e) {
      if (!(e instanceof ApiDisabledError)) setM(null);
    }
  }, []);

  useEffect(() => {
    void load();
    const id = setInterval(() => void load(), 10_000);
    return () => clearInterval(id);
  }, [load]);

  if (!m) return null;

  const req = m.search_requests ?? {};
  const idx = m.index ?? {};
  const runs = m.index_runs ?? {};
  const avg = m.index_avg_duration_ms ?? 0;
  const last = m.last_index_at
    ? new Date(m.last_index_at * 1000).toLocaleString()
    : t("admin.codeSearchNever");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Search className="h-4 w-4" />
          {t("admin.codeSearchTitle")}
        </CardTitle>
        <CardDescription>{t("admin.codeSearchHint")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label={t("admin.codeSearchBackend")} value={m.remote ? `${m.backend} → remote` : m.backend} />
          <Stat label={t("admin.codeSearchRepos")} value={idx.repos ?? 0} />
          <Stat label={t("admin.codeSearchDocs")} value={idx.documents ?? 0} />
          <Stat label={t("admin.codeSearchDirty")} value={idx.dirty ?? 0} />
          <Stat label={t("admin.codeSearchRequestsIndex")} value={req.index ?? 0} />
          <Stat label={t("admin.codeSearchRequestsGrep")} value={req.grep ?? 0} />
          <Stat label={t("admin.codeSearchRequestsIndexing")} value={req.indexing ?? 0} />
          <Stat label={t("admin.codeSearchRunsFull")} value={runs.full ?? 0} />
          <Stat label={t("admin.codeSearchRunsIncr")} value={runs.incremental ?? 0} />
          <Stat label={t("admin.codeSearchRunsError")} value={runs.error ?? 0} />
          <Stat label={t("admin.codeSearchAvg")} value={`${Math.round(avg)} ms`} />
          <Stat label={t("admin.codeSearchLastRun")} value={last} />
        </div>
        {m.index_error ? (
          <p className="text-sm text-destructive">
            {t("admin.codeSearchIndexError")}: {m.index_error}
          </p>
        ) : null}
        {m.backend === "grep" ? (
          <p className="text-sm text-muted-foreground">{t("admin.codeSearchDisabledHint")}</p>
        ) : null}
      </CardContent>
    </Card>
  );
}
