import { ShieldCheck } from "lucide-react";

import type { MergeGate } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/i18n";

const OK = "gap-1 text-green-600 dark:text-green-400";
const BLOCKED = "gap-1 text-amber-600 dark:text-amber-400";

/** 合并门禁徽章：approvals 与（可选的）CI 状态。 */
export function MergeGateBadges({ gate }: { gate: MergeGate }) {
  const { t } = useI18n();
  return (
    <div className="flex flex-wrap items-center gap-1">
      {gate.required > 0 && (
        <Badge variant="outline" className={gate.approvals >= gate.required ? OK : BLOCKED}>
          <ShieldCheck className="h-3 w-3" />
          {t("pulls.gateStatus", { approvals: gate.approvals, required: gate.required })}
        </Badge>
      )}
      {gate.ci_required && (
        <Badge variant="outline" className={gate.ci_status === "success" ? OK : BLOCKED}>
          {t(`pulls.ci.${gate.ci_status ?? "pending"}`)}
        </Badge>
      )}
    </div>
  );
}
