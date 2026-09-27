import { lazy, Suspense } from "react";
import { ShieldAlert } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

// 复用个人设置里的 MFA 区块（按需加载 qrcode 等依赖，避免拖大首屏）。
const MFASection = lazy(() =>
  import("@/pages/profile/MFASection").then((m) => ({ default: m.MFASection })),
);

/**
 * MFARequiredGate：实例强制 MFA 时，未启用 MFA 的用户被拦在此页，
 * 可就地完成 TOTP / 邮箱验证码绑定；启用成功后 onChanged 会重新拉取 /me。
 */
export function MFARequiredGate({ onChanged, onLogout }: { onChanged: () => void; onLogout: () => void }) {
  const { t } = useI18n();
  return (
    <div className="mx-auto max-w-2xl space-y-4 px-4 py-10">
      <Card className="border-amber-500/60">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldAlert className="h-4 w-4 text-amber-600" />
            {t("mfaGate.title")}
          </CardTitle>
          <CardDescription>{t("mfaGate.desc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{t("mfaGate.hint")}</p>
        </CardContent>
      </Card>
      <Suspense fallback={<Skeleton className="h-40 w-full" />}>
        <MFASection mfaEnabled={false} onChanged={onChanged} />
      </Suspense>
      <div className="text-center">
        <Button variant="ghost" size="sm" onClick={onLogout}>
          {t("app.logout")}
        </Button>
      </div>
    </div>
  );
}
