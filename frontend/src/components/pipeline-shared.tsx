import type { ReactNode } from "react";
import { Ban, CheckCircle2, Loader2, PauseCircle, XCircle } from "lucide-react";
import type { PipelineGraph, PipelineRunStatus } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

/** 运行状态徽标（pending/running/success/failed/cancelled）。 */
export function StatusBadge({ status }: { status: PipelineRunStatus }) {
  const { t } = useI18n();
  const map: Record<PipelineRunStatus, { icon: ReactNode; cls: string }> = {
    pending: { icon: <PauseCircle className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
    running: { icon: <Loader2 className="h-3 w-3 animate-spin" />, cls: "border-blue-600/40 text-blue-600" },
    success: { icon: <CheckCircle2 className="h-3 w-3" />, cls: "border-green-600/40 text-green-600" },
    failed: { icon: <XCircle className="h-3 w-3" />, cls: "border-destructive/50 text-destructive" },
    cancelled: { icon: <Ban className="h-3 w-3" />, cls: "border-muted-foreground/40 text-muted-foreground" },
  };
  const s = map[status] ?? map.pending;
  return (
    <Badge variant="outline" className={cn("gap-1", s.cls)}>
      {s.icon}
      {t(`pipeline.status.${status}`)}
    </Badge>
  );
}

/** 把流水线图转为 Mermaid flowchart（mermaid 内部用 dagre 布局引擎排版，与 D2 默认引擎一致）。 */
export function toMermaid(g: PipelineGraph): string {
  const id = (s: string) => "n" + s.replace(/[^a-zA-Z0-9]/g, "_");
  const esc = (s: string) => s.replace(/"/g, "'");
  const lines = ["flowchart TD"];
  const terminals: string[] = [];
  const parallels: string[] = [];
  for (const n of g.graph.nodes) {
    const lid = id(n.id);
    const label = n.when ? `${n.label} (when: ${n.when})` : n.label;
    if (n.kind === "start" || n.kind === "end") {
      lines.push(`  ${lid}(["${esc(n.label)}"])`);
      terminals.push(lid);
    } else if (n.kind === "parallel") {
      lines.push(`  ${lid}{{"${esc(label)}"}}`);
      parallels.push(lid);
    } else {
      lines.push(`  ${lid}["${esc(label)}"]`);
    }
  }
  for (const e of g.graph.edges) lines.push(`  ${id(e.from)} --> ${id(e.to)}`);
  if (terminals.length) {
    lines.push("  classDef gitdashTerminal fill:#6b7280,stroke:#374151,color:#fff;");
    lines.push(`  class ${terminals.join(",")} gitdashTerminal;`);
  }
  if (parallels.length) {
    lines.push("  classDef gitdashParallel fill:#2563eb,stroke:#1e40af,color:#fff;");
    lines.push(`  class ${parallels.join(",")} gitdashParallel;`);
  }
  return lines.join("\n");
}
