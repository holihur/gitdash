import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { toast } from "sonner";
import { MessageSquarePlus, Send } from "lucide-react";
import { api } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { MarkdownEditor } from "@/components/markdown-editor";

/** 浮动按钮位置在 localStorage 中的键名。 */
const POS_KEY = "gitdash-feedback-pos";
/** 拖拽判定阈值：位移小于该值视为点击。 */
const DRAG_THRESHOLD = 4;
/** 距离视口边缘的最小间距。 */
const EDGE_MARGIN = 8;

type Pos = { x: number; y: number };

function readStoredPos(): Pos | null {
  try {
    const raw = localStorage.getItem(POS_KEY);
    if (!raw) return null;
    const p = JSON.parse(raw) as Partial<Pos>;
    if (typeof p?.x === "number" && typeof p?.y === "number") return { x: p.x, y: p.y };
  } catch {
    /* 忽略解析 / 存储不可用 */
  }
  return null;
}

function storePos(p: Pos) {
  try {
    localStorage.setItem(POS_KEY, JSON.stringify(p));
  } catch {
    /* 忽略存储不可用 */
  }
}

/** 把坐标限制在视口内（留出边缘间距）。 */
function clampPos(x: number, y: number, el: HTMLElement | null): Pos {
  const w = el?.offsetWidth ?? 0;
  const h = el?.offsetHeight ?? 0;
  const maxX = Math.max(EDGE_MARGIN, window.innerWidth - w - EDGE_MARGIN);
  const maxY = Math.max(EDGE_MARGIN, window.innerHeight - h - EDGE_MARGIN);
  return {
    x: Math.min(Math.max(x, EDGE_MARGIN), maxX),
    y: Math.min(Math.max(y, EDGE_MARGIN), maxY),
  };
}

/**
 * 全局反馈组件：管理员启用后，在页面右下角显示浮动按钮，
 * 点击输入内容并通过后端提交到配置的远端仓库创建 Issue。
 *
 * 按钮支持拖放移动位置，位置保存在 localStorage；拖动不会触发打开弹窗。
 */
export function FeedbackWidget() {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(false);

  const [pos, setPos] = useState<Pos | null>(readStoredPos);
  const [dragging, setDragging] = useState(false);
  const btnRef = useRef<HTMLButtonElement>(null);
  const posRef = useRef<Pos | null>(pos);
  const drag = useRef<{ sx: number; sy: number; ox: number; oy: number; moved: boolean } | null>(null);
  // onClick 在 pointerup 之后触发，需要跨事件记住“刚刚是拖拽”。
  const justDragged = useRef(false);

  const updatePos = useCallback((next: Pos) => {
    posRef.current = next;
    setPos(next);
  }, []);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const cfg = await api.feedbackConfig();
        if (alive) setEnabled(!!cfg.enabled);
      } catch {
        /* 未启用 / 接口不可用：静默忽略 */
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

  // 首次挂载与窗口尺寸变化时，把已保存/拖拽后的位置重新限制在视口内。
  useEffect(() => {
    const reclamp = () => {
      if (posRef.current) updatePos(clampPos(posRef.current.x, posRef.current.y, btnRef.current));
    };
    reclamp();
    window.addEventListener("resize", reclamp);
    return () => window.removeEventListener("resize", reclamp);
  }, [enabled, updatePos]);

  if (!enabled) return null;

  const submit = async () => {
    const body = content.trim();
    if (!body || busy) return;
    setBusy(true);
    try {
      const r = await api.submitFeedback({ body, url: window.location.href });
      toast.success(t("feedback.submitted", { number: r.number }));
      setContent("");
      setOpen(false);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const onPointerDown = (e: ReactPointerEvent<HTMLButtonElement>) => {
    if (e.pointerType === "mouse" && e.button !== 0) return; // 仅左键
    const rect = e.currentTarget.getBoundingClientRect();
    const origin = posRef.current ?? { x: rect.left, y: rect.top };
    drag.current = { sx: e.clientX, sy: e.clientY, ox: origin.x, oy: origin.y, moved: false };
    justDragged.current = false;
    updatePos(origin); // 从 bottom-right 切换到 left/top，位置不跳变
    setDragging(true);
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {
      /* 部分环境（如 jsdom）不支持 */
    }
  };

  const onPointerMove = (e: ReactPointerEvent<HTMLButtonElement>) => {
    const d = drag.current;
    if (!d) return;
    const dx = e.clientX - d.sx;
    const dy = e.clientY - d.sy;
    if (!d.moved && Math.abs(dx) <= DRAG_THRESHOLD && Math.abs(dy) <= DRAG_THRESHOLD) return;
    d.moved = true;
    updatePos(clampPos(d.ox + dx, d.oy + dy, e.currentTarget));
  };

  const endDrag = (e: ReactPointerEvent<HTMLButtonElement>) => {
    const d = drag.current;
    drag.current = null;
    setDragging(false);
    try {
      e.currentTarget.releasePointerCapture(e.pointerId);
    } catch {
      /* ignore */
    }
    if (d?.moved) {
      justDragged.current = true;
      if (posRef.current) storePos(posRef.current);
    }
  };

  const onClick = () => {
    if (justDragged.current) {
      justDragged.current = false;
      return;
    }
    setOpen(true);
  };

  return (
    <>
      <Button
        ref={btnRef}
        className={cn(
          "fixed z-40 h-12 rounded-full px-5 shadow-lg touch-none select-none",
          pos ? "" : "bottom-6 right-6",
          dragging ? "cursor-grabbing" : "cursor-grab",
        )}
        style={pos ? { left: pos.x, top: pos.y } : undefined}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onClick={onClick}
        title={t("feedback.button")}
      >
        <MessageSquarePlus className="h-5 w-5" />
        <span className="hidden sm:inline">{t("feedback.button")}</span>
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("feedback.title")}</DialogTitle>
            <DialogDescription>{t("feedback.hint")}</DialogDescription>
          </DialogHeader>
          <MarkdownEditor
            id="feedback-content"
            value={content}
            onChange={setContent}
            placeholder={t("feedback.placeholder")}
            rows={6}
            maxLength={8000}
            autoFocus
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button onClick={submit} disabled={busy || !content.trim()}>
              <Send className="h-4 w-4" />
              {busy ? t("feedback.sending") : t("feedback.submit")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
