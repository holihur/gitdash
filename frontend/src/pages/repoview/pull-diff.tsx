import { useEffect, useState } from "react";
import { api, type MergeGate, type PullDiff } from "@/lib/api";
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
      files={diff.files}
      patch={diff.patch}
      canWrite={canWrite}
    />
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

