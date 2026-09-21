import { useCallback, useEffect, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { BookOpen, CheckCircle2, Circle, FolderGit2, KeyRound, Link2, Rocket, X } from "lucide-react";
import { api } from "@/lib/api";
import { useDocsUrl } from "@/lib/docs";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

const DISMISS_KEY = "gitdash.gettingStartedDismissed";

interface Step {
  key: string;
  done: boolean;
  to?: string;
  href?: string;
  icon: ReactNode;
}

/**
 * 新用户引导清单：展示在仓库首页，按实际状态标记完成项，可关闭。
 * 关闭状态存 localStorage，按浏览器记住。
 */
export function GettingStarted({ hasRepos }: { hasRepos: boolean }) {
  const { t } = useI18n();
  const docs = useDocsUrl();
  // 直接以 localStorage 初始化，避免首次渲染闪现已关闭的清单。
  const [dismissed, setDismissed] = useState(() => {
    try {
      return localStorage.getItem(DISMISS_KEY) === "1";
    } catch {
      return false;
    }
  });
  const [hasKeys, setHasKeys] = useState<boolean | null>(null);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const keys = await api.listKeys();
        if (alive) setHasKeys(keys.length > 0);
      } catch {
        if (alive) setHasKeys(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

  const dismiss = useCallback(() => {
    setDismissed(true);
    try {
      localStorage.setItem(DISMISS_KEY, "1");
    } catch {
      /* ignore */
    }
  }, []);

  if (dismissed) return null;
  // 已建仓库但公钥状态未知时先不渲染，避免老用户看到清单“闪现后消失”。
  if (hasRepos && hasKeys === null) return null;
  // 已经建了仓库、也加了公钥 → 视为已完成引导，自动隐藏（避免打扰老用户）。
  if (hasRepos && hasKeys) return null;

  const steps: Step[] = [
    { key: "repo", done: hasRepos, to: "/", icon: <FolderGit2 className="h-4 w-4" /> },
    { key: "ssh", done: hasKeys === true, to: "/keys", icon: <KeyRound className="h-4 w-4" /> },
    { key: "account", done: false, to: "/profile", icon: <Link2 className="h-4 w-4" /> },
  ];
  const doneCount = steps.filter((s) => s.done).length;

  return (
    <Card className="border-primary/30 bg-primary/5">
      <CardHeader className="flex-row items-start justify-between gap-2 space-y-0 pb-2">
        <CardTitle className="flex min-w-0 flex-wrap items-center gap-2 text-base">
          <Rocket className="h-4 w-4 shrink-0 text-primary" />
          {t("onboarding.title")}
          <span className="shrink-0 text-xs font-normal text-muted-foreground">
            {t("onboarding.progress", { done: doneCount, total: steps.length })}
          </span>
        </CardTitle>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          title={t("onboarding.dismiss")}
          onClick={dismiss}
        >
          <X className="h-4 w-4" />
        </Button>
      </CardHeader>
      <CardContent className="grid gap-2">
        <p className="text-sm text-muted-foreground">{t("onboarding.hint")}</p>
        <ul className="grid gap-1.5">
          {steps.map((s) => (
            <li key={s.key} className="flex items-start gap-2 text-sm">
              {s.done ? (
                <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-green-600" />
              ) : (
                <Circle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
              )}
              <span className="mt-0.5 shrink-0">{s.icon}</span>
              {s.to ? (
                <Link to={s.to} className="underline-offset-4 hover:text-primary hover:underline">
                  {t(`onboarding.step.${s.key}`)}
                </Link>
              ) : (
                <span>{t(`onboarding.step.${s.key}`)}</span>
              )}
            </li>
          ))}
        </ul>
        <div className="flex flex-wrap items-center gap-2 pt-1">
          {docs && (
            <Button variant="outline" size="sm" className="gap-2" asChild>
              <a href={docs} target="_blank" rel="noreferrer">
                <BookOpen className="h-4 w-4" />
                {t("onboarding.readDocs")}
              </a>
            </Button>
          )}
          <Button variant="ghost" size="sm" onClick={dismiss}>
            {t("onboarding.skip")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
