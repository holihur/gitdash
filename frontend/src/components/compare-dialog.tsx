import { useEffect, useMemo, useState } from "react";
import { GitCompare } from "lucide-react";
import { toast } from "sonner";
import { api, type Branch, type Tag } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DiffView, type DiffFileInfo } from "@/components/diff-view";

export function CompareDialog({
  owner,
  name,
  branches,
  tags,
  defaultRef,
  open,
  onOpenChange,
}: {
  owner: string;
  name: string;
  branches: Branch[];
  tags: Tag[];
  defaultRef: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, to } = useI18n();
  const [base, setBase] = useState("");
  const [head, setHead] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ files: DiffFileInfo[]; patch: string } | null>(null);

  const options = useMemo(
    () => [...branches.map((b) => b.name), ...tags.map((tg) => tg.name)],
    [branches, tags],
  );

  useEffect(() => {
    if (!open) return;
    setResult(null);
    const def = defaultRef || branches[0]?.name || "";
    setBase((cur) => cur || branches[0]?.name || def);
    setHead((cur) => cur || def);
  }, [open, branches, defaultRef]);

  const run = async () => {
    if (!base || !head || base === head) return;
    setBusy(true);
    try {
      const r = await api.compare(owner, name, base, head);
      setResult({ files: r.files, patch: r.patch });
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-4xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitCompare className="h-4 w-4" />
            {t("compare.title")}
          </DialogTitle>
          <DialogDescription>{t("compare.hint")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("compare.base")}
            <select
              className="h-9 min-w-40 rounded-md border bg-background px-2 text-sm"
              value={base}
              onChange={(e) => setBase(e.target.value)}
            >
              {options.map((o) => (
                <option key={o} value={o}>
                  {o}
                </option>
              ))}
            </select>
          </label>
          <span className="pb-2 text-muted-foreground">→</span>
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("compare.head")}
            <select
              className="h-9 min-w-40 rounded-md border bg-background px-2 text-sm"
              value={head}
              onChange={(e) => setHead(e.target.value)}
            >
              {options.map((o) => (
                <option key={o} value={o}>
                  {o}
                </option>
              ))}
            </select>
          </label>
          <Button
            size="sm"
            className="gap-1.5"
            disabled={busy || !base || !head || base === head}
            onClick={run}
          >
            <GitCompare className="h-3.5 w-3.5" />
            {t("compare.run")}
          </Button>
        </div>
        {result &&
          (result.files.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t("compare.empty")}
            </p>
          ) : (
            <DiffView files={result.files} patch={result.patch} />
          ))}
      </DialogContent>
    </Dialog>
  );
}

