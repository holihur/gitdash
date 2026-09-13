import { useCallback, useEffect, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { Bot, Loader2, Play, Plus, RefreshCw, Square, Terminal, Trash2, XCircle } from "lucide-react";
import { api, type ByokKey, type CopilotSession, type CopilotStatus } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import ConfirmDialog from "@/components/confirm-dialog";

export interface CopilotTabProps {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
}

function StatusBadge({ status }: { status: CopilotStatus }) {
  const { t } = useI18n();
  const map: Record<CopilotStatus, { icon: ReactNode; cls: string }> = {
    created: { icon: <Loader2 className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
    running: { icon: <Loader2 className="h-3 w-3 animate-spin" />, cls: "border-blue-600/40 text-blue-600" },
    stopped: { icon: <Square className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
    failed: { icon: <XCircle className="h-3 w-3" />, cls: "border-destructive/50 text-destructive" },
  };
  const s = map[status] ?? map.created;
  return (
    <Badge variant="outline" className={cn("gap-1", s.cls)}>
      {s.icon}
      {t(`copilot.status.${status}`)}
    </Badge>
  );
}

export default function CopilotTab({ owner, name, role }: CopilotTabProps) {
  const { t, to, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const canWrite = role === "owner" || role === "write";

  const [sessions, setSessions] = useState<CopilotSession[]>([]);
  const [byokKeys, setByokKeys] = useState<ByokKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [byokId, setByokId] = useState(0);
  const [image, setImage] = useState("");
  const [prompt, setPrompt] = useState("");
  const [command, setCommand] = useState("");
  const [expanded, setExpanded] = useState<number | null>(null);
  const [detail, setDetail] = useState<CopilotSession | null>(null);
  const [pendingDelete, setPendingDelete] = useState<CopilotSession | null>(null);

  const load = useCallback(async () => {
    try {
      const [ss, keys] = await Promise.all([api.listCopilots(owner, name), api.listByok()]);
      setSessions(ss);
      setByokKeys(keys);
      if (keys.length > 0 && byokId === 0) setByokId(keys[0].id);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [owner, name, to, byokId]);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 有运行中的会话时轮询
  useEffect(() => {
    const active = sessions.some((s) => s.status === "running" || s.status === "created");
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
    setImage("");
    setPrompt("");
    setCommand("");
    setCreateOpen(true);
  };

  const create = async () => {
    if (!byokId) return;
    setBusy(true);
    try {
      const s = await api.createCopilot(owner, name, {
        byok_id: byokId,
        image: image.trim() || undefined,
        prompt: prompt.trim(),
        command: command.trim() || undefined,
      });
      toast.success(t("copilot.launch"));
      setCreateOpen(false);
      setSessions((prev) => [s, ...prev]);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const start = async (s: CopilotSession) => {
    setBusy(true);
    try {
      const updated = await api.startCopilot(owner, name, s.id);
      setSessions((prev) => prev.map((x) => (x.id === s.id ? updated : x)));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const stop = async (s: CopilotSession) => {
    setBusy(true);
    try {
      const updated = await api.stopCopilot(owner, name, s.id);
      setSessions((prev) => prev.map((x) => (x.id === s.id ? updated : x)));
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
      setPendingDelete(null);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const toggleDetail = async (s: CopilotSession) => {
    if (expanded === s.id) {
      setExpanded(null);
      setDetail(null);
      return;
    }
    setExpanded(s.id);
    setDetail(null);
    try {
      setDetail(await api.getCopilot(owner, name, s.id));
    } catch {
      /* ignore */
    }
  };

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
        <CardContent>
          {byokKeys.length === 0 && (
            <p className="rounded-lg border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">
              {t("copilot.noByok")}
            </p>
          )}
        </CardContent>
      </Card>

      {loading ? (
        <Skeleton className="h-32 w-full" />
      ) : sessions.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">{t("copilot.empty")}</p>
      ) : (
        <div className="space-y-3">
          {sessions.map((s) => (
            <Card key={s.id}>
              <CardContent className="pt-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{t("copilot.session", { id: s.id })}</span>
                      <StatusBadge status={s.status} />
                    </div>
                    <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{s.image}</p>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">
                      {t("copilot.createdBy", { user: s.created_by })} · {t("byok.title")} #{s.byok_id} ·{" "}
                      {formatDate(s.created_at, locale)}
                    </p>
                    {s.prompt && <p className="mt-2 line-clamp-2 text-sm">{s.prompt}</p>}
                    {s.error && <p className="mt-1 text-xs text-destructive">{s.error}</p>}
                  </div>
                  {canWrite && (
                    <div className="flex shrink-0 items-center gap-1">
                      {s.status !== "running" && (
                        <Button size="sm" variant="outline" disabled={busy} onClick={() => start(s)}>
                          <Play className="h-4 w-4" />
                          {t("copilot.start")}
                        </Button>
                      )}
                      {s.status === "running" && (
                        <Button size="sm" variant="outline" disabled={busy} onClick={() => stop(s)}>
                          <Square className="h-4 w-4" />
                          {t("copilot.stop")}
                        </Button>
                      )}
                      <Button size="icon" variant="ghost" disabled={busy} onClick={() => toggleDetail(s)}>
                        <Terminal className="h-4 w-4" />
                      </Button>
                      <Button size="icon" variant="ghost" onClick={() => setPendingDelete(s)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  )}
                </div>

                {expanded === s.id && (
                  <div className="mt-4">
                    <p className="mb-2 text-xs font-medium uppercase text-muted-foreground">{t("copilot.logs")}</p>
                    <pre className="max-h-72 overflow-auto rounded-lg bg-muted/40 p-3 font-mono text-xs leading-relaxed">
                      {detail?.log || "…"}
                    </pre>
                  </div>
                )}
              </CardContent>
            </Card>
          ))}
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
              <Label htmlFor="copilot-image">{t("copilot.image")}</Label>
              <Input id="copilot-image" value={image} onChange={(e) => setImage(e.target.value)} placeholder={t("copilot.imagePlaceholder")} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="copilot-prompt">{t("copilot.prompt")}</Label>
              <Textarea id="copilot-prompt" rows={3} value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder={t("copilot.promptPlaceholder")} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="copilot-command">{t("copilot.command")}</Label>
              <Input id="copilot-command" value={command} onChange={(e) => setCommand(e.target.value)} placeholder={t("copilot.commandPlaceholder")} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setCreateOpen(false)} disabled={busy}>
              {t("login.back")}
            </Button>
            <Button onClick={create} disabled={busy || !byokId}>
              <Play className="h-4 w-4" />
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
