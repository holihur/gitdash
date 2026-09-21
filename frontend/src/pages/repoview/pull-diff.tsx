import { useEffect, useState } from "react";
import { Users } from "lucide-react";
import { api, type CodeownersStatus, type MergeGate, type PullDiff } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/i18n";
import { PullDiffView as InlinePullDiffView } from "@/components/diff-view";
import { MergeGateBadges } from "@/components/merge-gate";

export function PullDiffView({
  owner,
  name,
  number,
  canWrite,
}: {
  owner: string;
  name: string;
  number: number;
  canWrite: boolean;
}) {
  const [diff, setDiff] = useState<PullDiff | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    let alive = true;
    api
      .pullDiff(owner, name, number)
      .then((d) => alive && setDiff(d))
      .catch((e) => alive && setErr(e instanceof Error ? e.message : String(e)));
    return () => {
      alive = false;
    };
  }, [owner, name, number]);
  if (err) return <p className="text-xs text-destructive">{err}</p>;
  if (!diff) return <p className="py-4 text-center text-xs text-muted-foreground">…</p>;
  return (
    <InlinePullDiffView
      owner={owner}
      name={name}
      number={number}
      files={diff.files ?? []}
      patch={diff.patch ?? ""}
      canWrite={canWrite}
    />
  );
}

/** CODEOWNERS 状态徽章：变更文件要求的 code owner 是否已批准。 */
export function CodeownersBadge({ owner, name, number }: { owner: string; name: string; number: number }) {
  const { t } = useI18n();
  const [status, setStatus] = useState<CodeownersStatus | null>(null);
  useEffect(() => {
    let alive = true;
    api
      .pullCodeowners(owner, name, number)
      .then((s) => alive && setStatus(s))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [owner, name, number]);
  if (!status || (status.owners ?? []).length === 0) return null;
  return (
    <Badge
      variant="outline"
      className={
        status.satisfied
          ? "gap-1 font-normal text-green-600 dark:text-green-400"
          : "gap-1 font-normal text-amber-600 dark:text-amber-400"
      }
    >
      <Users className="h-3 w-3" />
      {status.satisfied
        ? t("pulls.codeownersSatisfied", { owners: (status.owners ?? []).join(", ") })
        : t("pulls.codeownersMissing", { owners: (status.missing ?? []).join(", ") })}
    </Badge>
  );
}

/** 合并门禁徽章：GET reviews 的 gate 字段（approvals/required，是否可合并）。 */
export function MergeGateBadge({
  owner,
  name,
  number,
  refreshKey,
}: {
  owner: string;
  name: string;
  number: number;
  refreshKey?: number;
}) {
  const [gate, setGate] = useState<MergeGate | null>(null);

  useEffect(() => {
    let alive = true;
    api
      .listPullReviews(owner, name, number)
      .then((r) => alive && setGate(r.gate ?? null))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [owner, name, number, refreshKey]);

  if (!gate) return null;
  return <MergeGateBadges gate={gate} />;
}

