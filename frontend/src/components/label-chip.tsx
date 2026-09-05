import type { Label } from "@/lib/api";
import { cn } from "@/lib/utils";

/** 根据背景色返回可读的文字色（深/浅）。 */
export function labelTextColor(color: string): string {
  const hex = color.replace(/^#/, "").padEnd(6, "0");
  const r = parseInt(hex.slice(0, 2) || "0", 16);
  const g = parseInt(hex.slice(2, 4) || "0", 16);
  const b = parseInt(hex.slice(4, 6) || "0", 16);
  return 0.299 * r + 0.587 * g + 0.114 * b > 160 ? "#1f2937" : "#ffffff";
}

interface Props {
  label: Label;
  className?: string;
  title?: string;
  onClick?: () => void;
}

/** 彩色圆角标签 chip（背景为标签色，自动选择黑白文字）。 */
export default function LabelChip({ label, className, title, onClick }: Props) {
  return (
    <span
      className={cn(
        "inline-flex max-w-full items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
        onClick && "cursor-pointer",
        className,
      )}
      style={{ backgroundColor: "#" + label.color.replace(/^#/, ""), color: labelTextColor(label.color) }}
      title={title}
      onClick={onClick}
    >
      <span className="truncate">{label.name}</span>
    </span>
  );
}
