import { useCallback, useEffect, useState } from "react";
import { Plus, ShieldCheck, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type Branch, type BranchProtection } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

export function BranchProtectionsCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [prots, setProts] = useState<BranchProtection[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branch, setBranch] = useState("");
  const [minApprovals, setMinApprovals] = useState("0");
  const [requireCI, setRequireCI] = useState(false);
  const [blockDeletion, setBlockDeletion] = useState(true);
  const [blockForcePush, setBlockForcePush] = useState(true);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [p, b] = await Promise.all([
        api.listBranchProtections(owner, name),
        api.branches(owner, name),
      ]);
      setProts(p);
      setBranches(b);
      setBranch((cur) => cur || b[0]?.name || "");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const save = async () => {
    if (!branch.trim()) return;
    const n = Number.parseInt(minApprovals, 10);
    if (!Number.isFinite(n) || n < 0 || n > 100) {
      toast.error(t("repo.bpInvalidApprovals"));
      return;
    }
    setBusy(true);
    try {
      await api.setBranchProtection(owner, name, branch.trim(), {
        min_approvals: n,
        require_ci: requireCI,
        block_deletion: blockDeletion,
        block_force_push: blockForcePush,
      });
      toast.success(t("repo.bpSaved", { branch: branch.trim() }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (b: string) => {
    try {
      await api.deleteBranchProtection(owner, name, b);
      toast.success(t("repo.bpRemoved", { branch: b }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const ruleSummary = (p: BranchProtection) => {
    const parts: string[] = [];
    if (p.min_approvals > 0) parts.push(t("repo.bpMinApprovals", { count: p.min_approvals }));
    if (p.require_ci) parts.push(t("repo.bpRequireCI"));
    if (p.block_deletion) parts.push(t("repo.bpBlockDeletion"));
    if (p.block_force_push) parts.push(t("repo.bpBlockForcePush"));
    return parts.join(" · ") || t("repo.bpNoRules");
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" />
          {t("repo.branchProtection")}
        </CardTitle>
        <CardDescription>{t("repo.branchProtectionDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.bpBranch")}
            <select
              className="h-9 rounded-md border bg-background px-2 text-sm"
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
            >
              {branches.map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.bpMinApprovalsLabel")}
            <Input
              type="number"
              min={0}
              max={100}
              className="w-20"
              value={minApprovals}
              onChange={(e) => setMinApprovals(e.target.value)}
            />
          </label>
          <label className="flex items-center gap-1.5 pb-2 text-sm">
            <input
              type="checkbox"
              checked={requireCI}
              onChange={(e) => setRequireCI(e.target.checked)}
            />
            {t("repo.bpRequireCI")}
          </label>
          <label className="flex items-center gap-1.5 pb-2 text-sm">
            <input
              type="checkbox"
              checked={blockDeletion}
              onChange={(e) => setBlockDeletion(e.target.checked)}
            />
            {t("repo.bpBlockDeletion")}
          </label>
          <label className="flex items-center gap-1.5 pb-2 text-sm">
            <input
              type="checkbox"
              checked={blockForcePush}
              onChange={(e) => setBlockForcePush(e.target.checked)}
            />
            {t("repo.bpBlockForcePush")}
          </label>
          <Button size="sm" className="gap-1 pb-0 mb-0.5" disabled={busy || !branch} onClick={save}>
            <Plus className="h-3.5 w-3.5" />
            {t("repo.bpSave")}
          </Button>
        </div>

        {prots.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("repo.bpEmpty")}</p>
        ) : (
          <div className="space-y-1.5">
            {prots.map((p) => (
              <div
                key={p.branch}
                className="flex items-center justify-between gap-2 rounded-md border bg-muted/20 p-2 text-sm"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{p.branch}</code>
                  <span className="text-xs text-muted-foreground">{ruleSummary(p)}</span>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive hover:text-destructive"
                  onClick={() => remove(p.branch)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

