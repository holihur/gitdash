import { BadgeCheck, ShieldAlert, ShieldQuestion } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/**
 * 提交签名标识（GPG / PGP）：
 * - verified：绿色，显示签名者用户名；
 * - unknown_key：琥珀色「未注册密钥」；
 * - invalid：红色「签名无效」；
 * - 未签名（无任何值）：不渲染。
 */
export function CommitVerifiedBadge({
  verified,
  status,
  className,
  compact,
}: {
  verified?: string;
  status?: "verified" | "unknown_key" | "invalid" | string;
  className?: string;
  /** 仅渲染图标（用于 blame 等窄空间）。 */
  compact?: boolean;
}) {
  const { t } = useI18n();
  if (verified) {
    if (compact) {
      return (
        <BadgeCheck
          className={cn("h-3 w-3 shrink-0 text-green-600", className)}
          aria-label={t("commits.gpgSigned", { user: verified })}
        />
      );
    }
    return (
      <Badge
        variant="outline"
        className={cn("shrink-0 gap-1 border-green-600/40 text-green-600", className)}
        title={t("commits.gpgSigned", { user: verified })}
      >
        <BadgeCheck className="h-3 w-3" />
        {verified}
      </Badge>
    );
  }
  if (status === "unknown_key") {
    if (compact) {
      return (
        <ShieldQuestion
          className={cn("h-3 w-3 shrink-0 text-amber-600 dark:text-amber-400", className)}
          aria-label={t("commits.gpgUnknownKey")}
        />
      );
    }
    return (
      <Badge
        variant="outline"
        className={cn("shrink-0 gap-1 text-amber-600 dark:text-amber-400", className)}
        title={t("commits.gpgUnknownKey")}
      >
        <ShieldQuestion className="h-3 w-3" />
        {t("commits.gpgUnknownKeyShort")}
      </Badge>
    );
  }
  if (status === "invalid") {
    if (compact) {
      return (
        <ShieldAlert
          className={cn("h-3 w-3 shrink-0 text-destructive", className)}
          aria-label={t("commits.gpgInvalid")}
        />
      );
    }
    return (
      <Badge
        variant="outline"
        className={cn("shrink-0 gap-1 border-destructive/40 text-destructive", className)}
        title={t("commits.gpgInvalid")}
      >
        <ShieldAlert className="h-3 w-3" />
        {t("commits.gpgInvalidShort")}
      </Badge>
    );
  }
  return null;
}
