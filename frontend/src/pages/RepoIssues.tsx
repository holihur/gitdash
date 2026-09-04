import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { CheckCircle2, Circle, MessageSquare, Plus } from "lucide-react";
import { api, type Issue } from "@/lib/api";
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
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

export default function RepoIssues({ owner, name }: { owner: string; name: string }) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<number | null>(null);
  const [busyIds, setBusyIds] = useState<Set<number>>(new Set());

  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setIssues(await api.listIssues(owner, name));
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
    if (!title.trim()) return;
    setCreating(true);
    try {
      const it = await api.createIssue(owner, name, title.trim(), body.trim());
      toast.success(t("issues.created", { number: it.number }));
      setOpen(false);
      setTitle("");
      setBody("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setCreating(false);
    }
  };

  const setState = async (issue: Issue, state: "open" | "closed") => {
    setBusyIds((s) => new Set(s).add(issue.id));
    try {
      await api.setIssueState(owner, name, issue.number, state);
      toast.success(
        state === "closed"
          ? t("issues.stateClosed", { number: issue.number })
          : t("issues.stateOpen", { number: issue.number }),
      );
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusyIds((s) => {
        const n = new Set(s);
        n.delete(issue.id);
        return n;
      });
    }
  };

  const openCount = issues.filter((i) => i.state === "open").length;
  const closedCount = issues.length - openCount;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          <span className="font-medium text-foreground">{openCount}</span> {t("issues.open")} ·{" "}
          <span className="font-medium text-foreground">{closedCount}</span> {t("issues.closed")}
        </p>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button size="sm" className="gap-2">
              <Plus className="h-4 w-4" />
              {t("issues.new")}
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>{t("issues.newDialogTitle")}</DialogTitle>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="issue-title">{t("issues.titleLabel")}</Label>
                <Input
                  id="issue-title"
                  placeholder={t("issues.titlePlaceholder")}
                  maxLength={200}
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      create();
                    }
                  }}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="issue-body">{t("issues.bodyLabel")}</Label>
                <Textarea
                  id="issue-body"
                  rows={5}
                  placeholder={t("issues.bodyPlaceholder")}
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={create} disabled={creating || !title.trim()}>
                {t("issues.new")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {error && !loading && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {t("issues.loadFailed", { error })}
          </CardContent>
        </Card>
      )}

      {loading && <p className="py-10 text-center text-sm text-muted-foreground">…</p>}

      {!loading && !error && issues.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <MessageSquare className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("issues.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("issues.emptyHint")}</p>
          </CardContent>
        </Card>
      )}

      {!loading && issues.length > 0 && (
        <div className="divide-y divide-border overflow-hidden rounded-lg border bg-card">
          {issues.map((issue) => {
            const isOpen = issue.state === "open";
            const busy = busyIds.has(issue.id);
            const openDetail = expanded === issue.number;
            return (
              <div key={issue.id}>
                <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:gap-3">
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-center gap-2 text-left"
                    onClick={() => setExpanded(openDetail ? null : issue.number)}
                  >
                    {isOpen ? (
                      <Circle className="h-4 w-4 shrink-0 fill-green-500 text-green-600" />
                    ) : (
                      <CheckCircle2 className="h-4 w-4 shrink-0 text-muted-foreground" />
                    )}
                    <span
                      className={cn(
                        "truncate font-medium hover:underline",
                        !isOpen && "text-muted-foreground",
                      )}
                    >
                      {issue.title}
                    </span>
                  </button>
                  <div className="flex items-center gap-2 pl-6 sm:min-w-0 sm:pl-0">
                    <p className="min-w-0 flex-1 truncate text-xs text-muted-foreground sm:flex-none">
                      #{issue.number} ·{" "}
                      {isOpen
                        ? t("issues.openedOn", {
                            author: issue.author,
                            date: formatDate(issue.created_at, locale),
                          })
                        : t("issues.closedOn", {
                            author: issue.author,
                            date: formatDate(issue.closed_at ?? issue.updated_at, locale),
                          })}
                    </p>
                    <Button
                      size="sm"
                      variant="outline"
                      className="shrink-0"
                      disabled={busy}
                      onClick={() => setState(issue, isOpen ? "closed" : "open")}
                    >
                      {isOpen ? t("issues.close") : t("issues.reopen")}
                    </Button>
                  </div>
                </div>
                {openDetail && (
                  <div className="border-t bg-muted/30 px-4 py-3">
                    <p className="whitespace-pre-wrap break-words text-sm">
                      {issue.body.trim() || t("issues.noBody")}
                    </p>
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
