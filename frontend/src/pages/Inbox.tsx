import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import {
  Bell,
  Check,
  CheckCheck,
  CheckCircle2,
  Circle,
  GitMerge,
  GitPullRequest,
  GitPullRequestArrow,
  GitPullRequestClosed,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { api, type Notification } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { cn, formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

function notifIcon(n: Notification) {
  switch (n.action) {
    case "opened":
      return n.kind === "issue" ? (
        <Circle className="h-4 w-4 shrink-0 fill-green-500 text-green-600" />
      ) : (
        <GitPullRequest className="h-4 w-4 shrink-0 text-green-600" />
      );
    case "closed":
      return n.kind === "issue" ? (
        <CheckCircle2 className="h-4 w-4 shrink-0 text-muted-foreground" />
      ) : (
        <GitPullRequestClosed className="h-4 w-4 shrink-0 text-muted-foreground" />
      );
    case "reopened":
      return n.kind === "issue" ? (
        <RotateCcw className="h-4 w-4 shrink-0 text-muted-foreground" />
      ) : (
        <GitPullRequestArrow className="h-4 w-4 shrink-0 text-muted-foreground" />
      );
    case "merged":
      return <GitMerge className="h-4 w-4 shrink-0 text-purple-600" />;
  }
}

export default function Inbox({ onChanged }: { onChanged?: () => void }) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const navigate = useNavigate();
  const [items, setItems] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busyIds, setBusyIds] = useState<Set<number>>(new Set());

  const load = useCallback(async () => {
    try {
      setItems(await api.inbox());
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const unread = items.filter((n) => !n.read).length;

  const open = async (n: Notification) => {
    if (!n.read) {
      try {
        await api.inboxRead(n.id);
        setItems((xs) => xs.map((x) => (x.id === n.id ? { ...x, read: true } : x)));
        onChanged?.();
      } catch {
        /* 忽略：仍跳转 */
      }
    }
    navigate(`/repo/${n.owner}/${n.repo}?tab=${n.kind === "issue" ? "issues" : "pulls"}`);
  };

  const markRead = async (id: number) => {
    setBusyIds((s) => new Set(s).add(id));
    try {
      await api.inboxRead(id);
      setItems((xs) => xs.map((x) => (x.id === id ? { ...x, read: true } : x)));
      onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusyIds((s) => {
        const n = new Set(s);
        n.delete(id);
        return n;
      });
    }
  };

  const readAll = async () => {
    try {
      await api.inboxReadAll();
      setItems((xs) => xs.map((x) => ({ ...x, read: true })));
      onChanged?.();
      toast.success(t("inbox.allRead"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const remove = async (n: Notification) => {
    setBusyIds((s) => new Set(s).add(n.id));
    try {
      await api.inboxDelete(n.id);
      setItems((xs) => xs.filter((x) => x.id !== n.id));
      if (!n.read) onChanged?.();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusyIds((s) => {
        const nset = new Set(s);
        nset.delete(n.id);
        return nset;
      });
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("inbox.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("inbox.subtitle")}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="gap-2 sm:self-start"
          onClick={readAll}
          disabled={unread === 0}
        >
          <CheckCheck className="h-4 w-4" />
          {t("inbox.markAllRead")}
        </Button>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {t("inbox.loadFailed", { error })}
          </CardContent>
        </Card>
      )}

      {loading && <p className="py-10 text-center text-sm text-muted-foreground">…</p>}

      {!loading && !error && items.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <Bell className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("inbox.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("inbox.emptyHint")}</p>
          </CardContent>
        </Card>
      )}

      {!loading && items.length > 0 && (
        <div className="divide-y divide-border overflow-hidden rounded-lg border bg-card">
          {items.map((n) => {
            const busy = busyIds.has(n.id);
            const icon = notifIcon(n);
            return (
              <div
                key={n.id}
                className={cn(
                  "flex items-start gap-3 px-4 py-3 transition-colors",
                  !n.read && "bg-primary/5",
                )}
              >
                <button
                  type="button"
                  className="flex min-w-0 flex-1 items-start gap-3 text-left"
                  onClick={() => open(n)}
                >
                  <span className="mt-0.5 shrink-0">{icon}</span>
                  <span className="min-w-0 flex-1">
                    <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                      <span className="font-mono text-xs text-muted-foreground">
                        {n.owner}/{n.repo}
                      </span>
                      {!n.read && (
                        <span className="h-2 w-2 rounded-full bg-blue-500" title={t("inbox.unread")} />
                      )}
                    </span>
                    <span
                      className={cn(
                        "block truncate font-medium",
                        n.read && "text-muted-foreground",
                      )}
                    >
                      #{n.number} · {n.title}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {t(`inbox.${n.kind}.${n.action}`, { actor: n.actor })} ·{" "}
                      {formatDate(n.created_at, locale)}
                    </span>
                  </span>
                </button>
                <div className="flex shrink-0 items-center gap-1">
                  {!n.read && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      title={t("inbox.markRead")}
                      disabled={busy}
                      onClick={() => markRead(n.id)}
                    >
                      <Check className="h-4 w-4" />
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-destructive hover:text-destructive"
                    title={t("inbox.delete")}
                    disabled={busy}
                    onClick={() => remove(n)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
