import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import {
  Bot,
  GitBranch,
  Loader2,
  Plus,
  RefreshCw,
  Send,
  Square,
  Trash2,
  Wrench,
  XCircle,
} from "lucide-react";
import { api, type ByokKey, type CopilotMessage, type CopilotSession, type CopilotStatus } from "@/lib/api";
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
  const normalized: CopilotStatus = status === "created" || status === "stopped" ? "idle" : status;
  const map: Record<CopilotStatus, { icon: ReactNode; cls: string }> = {
    idle: { icon: <Square className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
    created: { icon: <Square className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
    stopped: { icon: <Square className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
    running: { icon: <Loader2 className="h-3 w-3 animate-spin" />, cls: "border-blue-600/40 text-blue-600" },
    failed: { icon: <XCircle className="h-3 w-3" />, cls: "border-destructive/50 text-destructive" },
  };
  const s = map[normalized] ?? map.idle;
  return (
    <Badge variant="outline" className={cn("gap-1", s.cls)}>
      {s.icon}
      {t(`copilot.status.${normalized}`)}
    </Badge>
  );
}

type ChatItem = {
  id: string;
  role: "user" | "assistant" | "tool" | "error";
  text: string;
  tool?: string;
  isError?: boolean;
  pending?: boolean;
};

let seq = 0;
const uid = () => `m${Date.now()}-${seq++}`;

function historyToItems(messages: CopilotMessage[]): ChatItem[] {
  const out: ChatItem[] = [];
  for (const m of messages) {
    if (m.text) out.push({ id: uid(), role: m.role, text: m.text });
    for (const tl of m.tools ?? []) {
      out.push({ id: uid(), role: "tool", tool: tl.name, text: tl.result || tl.input || "", isError: tl.is_error });
    }
  }
  return out;
}

function CopilotChat({ owner, name, session }: { owner: string; name: string; session: CopilotSession }) {
  const { t } = useI18n();
  const [items, setItems] = useState<ChatItem[]>([]);
  const [input, setInput] = useState("");
  const [running, setRunning] = useState(false);
  const [connected, setConnected] = useState(false);

  const wsRef = useRef<WebSocket | null>(null);
  const curAssistant = useRef<string | null>(null);
  const pendingTool = useRef<string | null>(null);

  const handle = useCallback((ev: Record<string, unknown>) => {
    const type = String(ev.type ?? "");
    if (type === "history") {
      setItems(historyToItems((ev.messages as CopilotMessage[]) ?? []));
      curAssistant.current = null;
      pendingTool.current = null;
      setRunning(false);
      return;
    }
    if (type === "status") {
      setRunning(ev.status === "running");
      return;
    }
    if (type === "delta") {
      const text = String(ev.text ?? "");
      setItems((prev) => {
        let id = curAssistant.current;
        if (!id) {
          id = uid();
          curAssistant.current = id;
          return [...prev, { id, role: "assistant", text }];
        }
        const idx = prev.findIndex((m) => m.id === id);
        if (idx < 0) return [...prev, { id, role: "assistant", text }];
        const copy = prev.slice();
        copy[idx] = { ...copy[idx], text: copy[idx].text + text };
        return copy;
      });
      return;
    }
    if (type === "tool_start") {
      curAssistant.current = null;
      const id = uid();
      pendingTool.current = id;
      setItems((prev) => [
        ...prev,
        { id, role: "tool", tool: String(ev.name ?? "tool"), text: String(ev.input ?? ""), pending: true },
      ]);
      return;
    }
    if (type === "tool_end") {
      const id = pendingTool.current;
      pendingTool.current = null;
      setItems((prev) => {
        const idx = id ? prev.findIndex((m) => m.id === id) : -1;
        const patch = {
          text: String(ev.result ?? ""),
          isError: Boolean(ev.is_error),
          pending: false,
        };
        if (idx < 0) {
          return [...prev, { id: uid(), role: "tool", tool: String(ev.name ?? "tool"), ...patch }];
        }
        const copy = prev.slice();
        copy[idx] = { ...copy[idx], ...patch };
        return copy;
      });
      return;
    }
    if (type === "done") {
      const finalText = String(ev.text ?? "");
      const id = curAssistant.current;
      curAssistant.current = null;
      setItems((prev) => {
        if (!id) return finalText ? [...prev, { id: uid(), role: "assistant", text: finalText }] : prev;
        const idx = prev.findIndex((m) => m.id === id);
        if (idx < 0) return [...prev, { id, role: "assistant", text: finalText }];
        const copy = prev.slice();
        copy[idx] = { ...copy[idx], text: finalText };
        return copy;
      });
      setRunning(false);
      return;
    }
    if (type === "error") {
      setItems((prev) => [...prev, { id: uid(), role: "error", text: String(ev.error ?? "error") }]);
      setRunning(false);
    }
  }, []);

  useEffect(() => {
    const ws = new WebSocket(api.copilotChatUrl(owner, name, session.id));
    wsRef.current = ws;
    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onerror = () => setConnected(false);
    ws.onmessage = (e) => {
      try {
        handle(JSON.parse(e.data));
      } catch {
        /* ignore malformed frame */
      }
    };
    return () => {
      wsRef.current = null;
      ws.close();
    };
  }, [owner, name, session.id, handle]);

  const send = () => {
    const text = input.trim();
    if (!text || running) return;
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      toast.error(t("copilot.notConnected"));
      return;
    }
    setInput("");
    ws.send(JSON.stringify({ type: "user", text }));
    setItems((prev) => [...prev, { id: uid(), role: "user", text }]);
    setRunning(true);
  };

  const cancel = () => wsRef.current?.send(JSON.stringify({ type: "cancel" }));

  return (
    <Card className="flex h-full min-h-0 flex-col">
      <CardHeader className="shrink-0 pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <Bot className="h-4 w-4" />
          {t("copilot.session", { id: session.id })}
          <StatusBadge status={running ? "running" : session.status} />
          {!connected && <span className="text-xs font-normal text-destructive">{t("copilot.disconnected")}</span>}
        </CardTitle>
        <CardDescription className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span>{t("copilot.createdBy", { user: session.created_by })}</span>
          {session.branch && (
            <span className="inline-flex items-center gap-1 font-mono text-xs">
              <GitBranch className="h-3 w-3" />
              {session.branch}
              {session.head_sha ? `@${session.head_sha.slice(0, 7)}` : ""}
            </span>
          )}
          {session.prompt && <span className="truncate">{session.prompt}</span>}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col gap-3">
        <div className="min-h-[22rem] flex-1 space-y-3 overflow-y-auto rounded-lg border bg-muted/20 p-3">
          {items.length === 0 && (
            <p className="py-8 text-center text-sm text-muted-foreground">{t("copilot.chatEmpty")}</p>
          )}
          {items.map((m) => (
            <div key={m.id} className={cn("flex", m.role === "user" ? "justify-end" : "justify-start")}>
              {m.role === "tool" ? (
                <div className="max-w-[90%] rounded-md border-l-2 border-muted-foreground/30 bg-background/60 px-3 py-1.5 font-mono text-xs text-muted-foreground">
                  <span className="inline-flex items-center gap-1 font-medium">
                    <Wrench className="h-3 w-3" />
                    {m.tool}
                    {m.pending && <Loader2 className="h-3 w-3 animate-spin" />}
                  </span>
                  {m.text && (
                    <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-all">{m.text}</pre>
                  )}
                </div>
              ) : (
                <div
                  className={cn(
                    "max-w-[85%] whitespace-pre-wrap break-words rounded-lg px-3 py-2 text-sm",
                    m.role === "user" && "bg-primary text-primary-foreground",
                    m.role === "assistant" && "bg-muted",
                    m.role === "error" && "border border-destructive/40 text-destructive",
                  )}
                >
                  {m.text}
                </div>
              )}
            </div>
          ))}
        </div>
        <div className="flex shrink-0 items-end gap-2">
          <Textarea
            rows={2}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                send();
              }
            }}
            placeholder={t("copilot.inputPlaceholder")}
            disabled={running}
            className="min-h-[44px] resize-none"
          />
          {running ? (
            <Button variant="destructive" onClick={cancel} className="h-11">
              <Square className="h-4 w-4" />
              {t("copilot.stop")}
            </Button>
          ) : (
            <Button onClick={send} disabled={!input.trim() || !connected} className="h-11">
              <Send className="h-4 w-4" />
              {t("copilot.send")}
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
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
  const [prompt, setPrompt] = useState("");
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
    setCreateOpen(true);
  };

  const create = async () => {
    if (!byokId) return;
    setBusy(true);
    try {
      const s = await api.createCopilot(owner, name, { byok_id: byokId, prompt: prompt.trim() });
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
                onClick={() => setActiveId(s.id)}
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
                  {t("copilot.createdBy", { user: s.created_by })} · {formatDate(s.created_at, locale)}
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
