import { useEffect, useState } from "react";
import { AlertTriangle, Info, ShieldAlert, X } from "lucide-react";
import { api, type Announcement } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/** 已关闭公告记录在 localStorage：值为公告 id（内容指纹），内容变化后重新展示。 */
const DISMISS_KEY = "gitdash-announcement-dismissed";

const LEVEL_STYLES = {
  info: {
    wrap: "border-primary/30 bg-primary/10 text-foreground",
    icon: Info,
  },
  warning: {
    wrap: "border-amber-500/40 bg-amber-500/10 text-foreground",
    icon: AlertTriangle,
  },
  critical: {
    wrap: "border-destructive/40 bg-destructive/10 text-foreground",
    icon: ShieldAlert,
  },
} as const;

/**
 * 全站通知：管理端发布的公告条，展示在所有页面（登录页与已登录界面）顶部。
 * 用户可关闭；关闭状态按公告内容指纹记忆，管理员更新内容后会再次展示。
 */
export function AnnouncementBanner() {
  const { t } = useI18n();
  const [data, setData] = useState<Announcement | null>(null);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    let alive = true;
    api
      .announcement()
      .then((d) => {
        if (!alive) return;
        let closed = false;
        if (d?.enabled && d.id) {
          try {
            closed = localStorage.getItem(DISMISS_KEY) === d.id;
          } catch {
            /* ignore */
          }
        }
        setData(d);
        setDismissed(closed);
      })
      .catch(() => {
        /* 公告不是关键路径，静默失败 */
      });
    return () => {
      alive = false;
    };
  }, []);

  if (!data?.enabled || dismissed) return null;

  const level = data.level && data.level in LEVEL_STYLES ? data.level : "info";
  const { wrap, icon: Icon } = LEVEL_STYLES[level];

  const dismiss = () => {
    if (data.id) {
      try {
        localStorage.setItem(DISMISS_KEY, data.id);
      } catch {
        /* ignore */
      }
    }
    setDismissed(true);
  };

  return (
    <div role="status" className={cn("border-b", wrap)}>
      <div className="container flex items-start gap-3 py-2 text-sm">
        <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
        <div className="min-w-0 flex-1 whitespace-pre-line break-words">
          {data.title && <span className="font-medium">{data.title}</span>}
          {data.title && data.message && <span className="mx-1.5 text-muted-foreground">·</span>}
          {data.message && <span>{data.message}</span>}
        </div>
        <button
          type="button"
          onClick={dismiss}
          title={t("announcement.dismiss")}
          aria-label={t("announcement.dismiss")}
          className="-mr-1 -mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-black/5 hover:text-foreground dark:hover:bg-white/10"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
