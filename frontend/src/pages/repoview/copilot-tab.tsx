import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import { Bot, GitBranch, Plus, RefreshCw, Trash2 } from "lucide-react";
import { api, type ByokKey, type CopilotSession } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { RelativeTime } from "@/components/relative-time";
import { apiErrorMsg } from "@/lib/errors";
import ConfirmDialog from "@/components/confirm-dialog";
import { StatusBadge } from "@/components/copilot/status-badge";
import { CopilotChat } from "@/components/copilot/chat";
import { canWrite as roleCanWrite } from "@/lib/repo-role";

export interface CopilotTabProps {
  owner: string;
  name: string;
  role?: string;
}

export default function CopilotTab({ owner, name, role }: CopilotTabProps) {
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
  const canWrite = roleCanWrite(role);
  const [searchParams, setSearchParams] = useSearchParams();
  const wantedId = Number(searchParams.get("copilot")) || null;

  const [sessions, setSessions] = useState<CopilotSession[]>([]);
  const [byokKeys, setByokKeys] = useState<ByokKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [byokId, setByokId] = useState(0);
  const [prompt, setPrompt] = useState("");
  const [issueRef, setIssueRef] = useState("");
  const [activeId, setActiveId] = useState<number | null>(null);
  const [pendingDelete, setPendingDelete] = useState<CopilotSession | null>(null);

  const load = useCallback(async () => {
    try {
      const [ss, keys] = await Promise.all([api.listCopilots(owner, name), api.listByok()]);
      setSessions(ss);
      setByokKeys(keys);
      setActiveId((prev) => prev ?? ss[0]?.id ?? null);
      if (keys.length > 0) setByokId((prev) => prev || keys[0].id);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 从 issue 页面跳转过来（?copilot=<id>）时聚焦对应会话。
  useEffect(() => {
    if (wantedId && sessions.some((s) => s.id === wantedId)) setActiveId(wantedId);
  }, [wantedId, sessions]);

  const selectSession = (id: number) => {
    setActiveId(id);
    if (searchParams.has("copilot")) {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.delete("copilot");
          return next;
        },
        { replace: true },
      );
    }
  };

  // 有运行中的会话时轮询状态
  useEffect(() => {
    const active = sessions.some((s) => s.status === "running");
    if (!active) return;
    const timer = window.setTimeout(() => {
      api
        .listCopilots(owner, name)
        .then(setSessions)
        .catch(() => undefined);
    }, 4000);
    return () => window.clearTimeout(timer);
  }, [sessions, owner, name]);

  const openCreate = () => {
    setByokId(byokKeys[0]?.id ?? 0);
    setPrompt("");
    setIssueRef("");
    setCreateOpen(true);
  };

  const create = async () => {
    if (!byokId) return;
    setBusy(true);
    try {
      const issueNumber = Number(issueRef.replace(/[^0-9]/g, "")) || 0;
      const s = await api.createCopilot(owner, name, {
        byok_id: byokId,
        prompt: prompt.trim(),
        issue_number: issueNumber || undefined,
      });
      toast.success(t("copilot.launch"));
      setCreateOpen(false);
      setSessions((prev) => [s, ...prev]);
      setActiveId(s.id);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (s: CopilotSession) => {
    try {
      await api.deleteCopilot(owner, name, s.id);
      toast.success(t("copilot.delete"));
      setSessions((prev) => prev.filter((x) => x.id !== s.id));
      setActiveId((prev) => (prev === s.id ? null : prev));
      setPendingDelete(null);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const active = sessions.find((s) => s.id === activeId) ?? null;

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-2">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <CardTitle className="flex items-center gap-2 text-base">
                <Bot className="h-4 w-4" />
                {t("copilot.title")}
              </CardTitle>
              <CardDescription className="mt-1">{t("copilot.hint")}</CardDescription>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button size="sm" variant="ghost" onClick={load}>
                <RefreshCw className="h-4 w-4" />
                {t("copilot.refresh")}
              </Button>
              {canWrite && (
                <Button size="sm" variant="outline" disabled={byokKeys.length === 0} onClick={openCreate}>
                  <Plus className="h-4 w-4" />
                  {t("copilot.new")}
                </Button>
              )}
            </div>
          </div>
        </CardHeader>
        {byokKeys.length === 0 && (
          <CardContent>
            <p className="rounded-lg border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">
              {t("copilot.noByok")}
            </p>
          </CardContent>
        )}
      </Card>

      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : sessions.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">{t("copilot.empty")}</p>
      ) : (
        <div className="grid gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
          <div className="space-y-2">
            {sessions.map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => selectSession(s.id)}
                className={cn(
                  "w-full rounded-lg border p-3 text-left transition-colors",
                  activeId === s.id ? "border-primary/60 bg-muted/50" : "hover:bg-muted/40",
                )}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium">{t("copilot.session", { id: s.id })}</span>
                  <StatusBadge status={s.status} />
                </div>
                {s.branch && (
                  <p className="mt-1 flex items-center gap-1 truncate font-mono text-xs text-muted-foreground">
                    <GitBranch className="h-3 w-3" />
                    {s.branch}
                  </p>
                )}
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("copilot.createdBy", { user: s.created_by })} · <RelativeTime iso={s.created_at} locale={locale} />
                </p>
                {s.prompt && <p className="mt-1 line-clamp-2 text-xs">{s.prompt}</p>}
                {s.error && <p className="mt-1 text-xs text-destructive">{s.error}</p>}
                {canWrite && (
                  <div className="mt-2 flex justify-end">
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-7 w-7"
                      onClick={(e) => {
                        e.stopPropagation();
                        setPendingDelete(s);
                      }}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                )}
              </button>
            ))}
          </div>
          <div className="min-h-[28rem]">
            {active ? (
              <CopilotChat key={active.id} owner={owner} name={name} session={active} />
            ) : (
              <div className="flex h-full items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
                {t("copilot.selectSession")}
              </div>
            )}
          </div>
        </div>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("copilot.new")}</DialogTitle>
            <DialogDescription>{t("copilot.hint")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="copilot-byok">{t("copilot.byok")}</Label>
              <select
                id="copilot-byok"
                className="rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                value={byokId}
                onChange={(e) => setByokId(Number(e.target.value))}
              >
                {byokKeys.map((k) => (
                  <option key={k.id} value={k.id}>
                    {k.name} ({k.model || "claude"})
                  </option>
                ))}
              </select>
              <p className="text-xs text-muted-foreground">{t("copilot.byokHint")}</p>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="copilot-issue">{t("copilot.issueNumber")}</Label>
              <Input
                id="copilot-issue"
                value={issueRef}
                onChange={(e) => setIssueRef(e.target.value)}
                placeholder={t("copilot.issueNumberPlaceholder")}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="copilot-prompt">{t("copilot.prompt")}</Label>
              <Textarea
                id="copilot-prompt"
                rows={3}
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder={t("copilot.promptPlaceholder")}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setCreateOpen(false)} disabled={busy}>
              {t("login.back")}
            </Button>
            <Button onClick={create} disabled={busy || !byokId}>
              <Plus className="h-4 w-4" />
              {t("copilot.launch")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        title={t("copilot.delete")}
        description={pendingDelete ? t("copilot.confirmDelete", { id: pendingDelete.id }) : ""}
        confirmText={t("copilot.delete")}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}
