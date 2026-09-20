import { useEffect, useState } from "react";
import { toast } from "sonner";
import { MessageSquarePlus, Send } from "lucide-react";
import { api } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";

/**
 * 全局反馈组件：管理员启用后，在页面右下角显示浮动按钮，
 * 点击输入内容并通过后端提交到配置的远端仓库创建 Issue。
 */
export function FeedbackWidget() {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(false);

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

  return (
    <>
      <Button
        className="fixed bottom-6 right-6 z-40 h-12 rounded-full px-5 shadow-lg"
        onClick={() => setOpen(true)}
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
          <Textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
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
