import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import {
  GitMerge,
  GitPullRequest,
  GitPullRequestArrow,
  GitPullRequestClosed,
  Plus,
} from "lucide-react";
import { api, type PullDiff, type PullRequest } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { cn, formatDate } from "@/lib/utils";

export default function RepoPulls({ owner, name }: { owner: string; name: string }) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [pulls, setPulls] = useState<PullRequest[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<number | null>(null);
  const [busy, setBusy] = useState<Set<number>>(new Set());

  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [source, setSource] = useState("");
  const [target, setTarget] = useState("");
  const [branches, setBranches] = useState<string[]>([]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [ps, bs] = await Promise.all([
        api.listPulls(owner, name),
        api.branches(owner, name),
      ]);
      setPulls(ps);
      setBranches(bs.map((b) => b.name));
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [owner, name]);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    if (!title.trim() || !source || !target) return;
    setBusy((s) => new Set(s).add(0));
    try {
      const pr = await api.createPull(owner, name, title.trim(), body.trim(), source, target);
      toast.success(t("pulls.created", { number: pr.number }));
      setOpen(false);
      load();
      setExpanded(pr.number);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy((s) => {
        const n = new Set(s);
        n.delete(0);
        return n;
      });
    }
  };

  const act = async (pr: PullRequest, fn: () => Promise<unknown>, msgKey: string) => {
    setBusy((s) => new Set(s).add(pr.id));
    try {
      await fn();
      toast.success(t(msgKey, { number: pr.number }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy((s) => {
        const n = new Set(s);
        n.delete(pr.id);
        return n;
      });
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t("pulls.count", { count: pulls.length })}</p>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button size="sm" className="gap-2">
              <Plus className="h-4 w-4" />
              {t("pulls.new")}
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>{t("pulls.newDialogTitle")}</DialogTitle>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="pr-title">{t("issues.titleLabel")}</Label>
                <Input
                  id="pr-title"
                  placeholder={t("pulls.titlePlaceholder")}
                  maxLength={200}
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="grid gap-2">
                  <Label htmlFor="pr-source">{t("pulls.source")}</Label>
                  <select
                    id="pr-source"
                    value={source}
                    onChange={(e) => setSource(e.target.value)}
                    className="h-10 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {branches.map((b) => (
                      <option key={b} value={b}>
                        {b}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="pr-target">{t("pulls.target")}</Label>
                  <select
                    id="pr-target"
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                    className="h-10 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {branches.map((b) => (
                      <option key={b} value={b}>
                        {b}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="pr-body">{t("issues.bodyLabel")}</Label>
                <Textarea
                  id="pr-body"
                  rows={4}
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={create} disabled={busy.has(0) || !title.trim() || !source || !target}>
                {t("pulls.new")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {loading && <p className="py-10 text-center text-sm text-muted-foreground">…</p>}

      {!loading && !error && pulls.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <GitPullRequest className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("pulls.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("pulls.emptyHint")}</p>
          </CardContent>
        </Card>
      )}

      {!loading && pulls.length > 0 && (
        <div className="divide-y divide-border overflow-hidden rounded-lg border bg-card">
          {pulls.map((pr) => {
            const busyId = busy.has(pr.id);
            const openDetail = expanded === pr.number;
            return (
              <div key={pr.id}>
                <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:gap-3">
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-center gap-2 text-left"
                    onClick={() => setExpanded(openDetail ? null : pr.number)}
                  >
                    {pr.state === "open" ? (
                      <GitPullRequest className="h-4 w-4 shrink-0 text-green-600" />
                    ) : pr.state === "merged" ? (
                      <GitMerge className="h-4 w-4 shrink-0 text-purple-600" />
                    ) : (
                      <GitPullRequestClosed className="h-4 w-4 shrink-0 text-muted-foreground" />
                    )}
                    <span
                      className={cn(
                        "truncate font-medium hover:underline",
                        pr.state !== "open" && "text-muted-foreground",
                      )}
                    >
                      {pr.title}
                    </span>
                  </button>
                  <div className="flex items-center gap-2 pl-6 sm:min-w-0 sm:pl-0">
                    <span className="text-xs text-muted-foreground">
                      #{pr.number} · {t("pulls.openedOn", { author: pr.author, date: formatDate(pr.created_at, locale) })}
                    </span>
                    <div className="ml-auto flex shrink-0 items-center gap-2">
                      <code className="hidden rounded bg-muted px-1.5 py-0.5 text-xs sm:inline">
                        {pr.source_branch}
                        <GitPullRequestArrow className="mx-0.5 inline h-3 w-3 text-muted-foreground" />
                        {pr.target_branch}
                      </code>
                      {pr.state === "open" && (
                        <>
                          <Button
                            size="sm"
                            className="gap-1"
                            disabled={busyId}
                            onClick={() =>
                              act(
                                pr,
                                () => api.mergePull(owner, name, pr.number),
                                "pulls.merged",
                              )
                            }
                          >
                            <GitMerge className="h-3.5 w-3.5" />
                            {t("pulls.merge")}
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={busyId}
                            onClick={() =>
                              act(
                                pr,
                                () => api.setPullState(owner, name, pr.number, "closed"),
                                "pulls.closed",
                              )
                            }
                          >
                            {t("pulls.close")}
                          </Button>
                        </>
                      )}
                      {pr.state === "closed" && (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busyId}
                          onClick={() =>
                            act(pr, () => api.setPullState(owner, name, pr.number, "open"), "pulls.reopened")
                          }
                        >
                          {t("pulls.reopen")}
                        </Button>
                      )}
                    </div>
                  </div>
                </div>
                {openDetail && (
                  <div className="space-y-3 border-t bg-muted/20 px-4 py-3">
                    {pr.body.trim() && (
                      <p className="whitespace-pre-wrap break-words text-sm">{pr.body}</p>
                    )}
                    {pr.state === "merged" && (
                      <p className="text-xs text-muted-foreground">
                        {t("pulls.mergedOn", {
                          by: pr.merged_by,
                          date: pr.merged_at ? formatDate(pr.merged_at, locale) : "",
                        })}
                      </p>
                    )}
                    <PullDiffView owner={owner} name={name} number={pr.number} />
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function PullDiffView({ owner, name, number }: { owner: string; name: string; number: number }) {
  const { t } = useI18n();
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
    <div className="space-y-2">
      <div className="flex flex-wrap gap-1.5">
        {diff.files.map((f) => (
          <Badge
            key={f.path}
            variant="outline"
            className="max-w-full font-mono text-xs"
            title={`${f.path} (+${f.insertions} -${f.deletions})`}
          >
            <span
              className={
                f.status === "A"
                  ? "text-green-600"
                  : f.status === "D"
                    ? "text-red-600"
                    : "text-yellow-600"
              }
            >
              {f.status}
            </span>
            <span className="mx-1 truncate">{f.path}</span>
            <span className="shrink-0 text-muted-foreground">
              +{f.insertions} -{f.deletions}
            </span>
          </Badge>
        ))}
      </div>
      {diff.patch ? (
        <div className="max-h-[60vh] overflow-auto rounded-md border bg-background/60">
          <DiffLines patch={diff.patch} />
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">{t("pulls.noChanges")}</p>
      )}
    </div>
  );
}

function DiffLines({ patch }: { patch: string }) {
  return (
    <pre className="min-w-max px-3 py-2 font-mono text-xs leading-5">
      {patch.split("\n").map((line, i) => {
        let cls = "";
        if (line.startsWith("+") && !line.startsWith("+++")) cls = "bg-green-500/15 text-green-700 dark:text-green-400";
        else if (line.startsWith("-") && !line.startsWith("---")) cls = "bg-red-500/15 text-red-700 dark:text-red-400";
        else if (line.startsWith("@")) cls = "text-blue-600 dark:text-blue-400";
        else if (line.startsWith("diff --git") || line.startsWith("index ")) cls = "text-muted-foreground";
        return (
          <div key={i} className={cls}>
            {line || " "}
          </div>
        );
      })}
    </pre>
  );
}
