import type { ReactNode } from "react";
import { Loader2, Square, XCircle } from "lucide-react";
import type { CopilotStatus } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export function StatusBadge({ status }: { status: CopilotStatus }) {
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

