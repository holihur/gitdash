import { useEffect, useRef, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Bot, GitBranch, Loader2, Send, Square, Wrench } from "lucide-react";
import { api, type CopilotMessage, type CopilotSession } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { StatusBadge } from "./status-badge";

export type ChatItem = {
  id: string;
  role: "user" | "assistant" | "tool" | "error";
  text: string;
  tool?: string;
  isError?: boolean;
  pending?: boolean;
};

let seq = 0;
const uid = () => `m${Date.now()}-${seq++}`;

export function historyToItems(messages: CopilotMessage[]): ChatItem[] {
  const out: ChatItem[] = [];
  for (const m of messages) {
    if (m.text) out.push({ id: uid(), role: m.role, text: m.text });
    for (const tl of m.tools ?? []) {
      out.push({ id: uid(), role: "tool", tool: tl.name, text: tl.result || tl.input || "", isError: tl.is_error });
    }
  }
  return out;
}

export function CopilotChat({ owner, name, session }: { owner: string; name: string; session: CopilotSession }) {
  const { t } = useI18n();
  const [items, setItems] = useState<ChatItem[]>([]);
  // 关联 issue 的新会话：把生成的起始提示词预填到输入框，用户确认后发送即可。
  const [input, setInput] = useState(() =>
    session.issue_number && session.prompt ? session.prompt : "",
  );
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
          {session.issue_number ? (
            <span className="text-xs">{t("copilot.linkedIssue", { number: session.issue_number })}</span>
          ) : null}
          {session.pr_number ? (
            <Link to={`/repo/${owner}/${name}/pulls`} className="text-xs text-primary hover:underline">
              {t("copilot.openedPull", { number: session.pr_number })}
            </Link>
          ) : null}
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

