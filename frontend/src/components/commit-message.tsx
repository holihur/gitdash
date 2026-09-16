import { useState } from "react";
import { cn } from "@/lib/utils";

/** 列表中提交信息的默认固定长度（超出部分折叠） */
export const COMMIT_MESSAGE_MAX_LENGTH = 48;

/**
 * 固定长度的提交信息：
 * - 超出长度时截断并追加省略号；
 * - 悬停通过 title 显示全文；
 * - 点击可展开/收起全文。
 */
export function CommitMessage({
  message,
  max = COMMIT_MESSAGE_MAX_LENGTH,
  className,
}: {
  message?: string;
  max?: number;
  className?: string;
}) {
  const [expanded, setExpanded] = useState(false);

  if (!message) return null;

  if (message.length <= max) {
    return (
      <span className={cn("min-w-0 flex-1 truncate", className)} title={message}>
        {message}
      </span>
    );
  }

  return (
    <button
      type="button"
      title={message}
      aria-expanded={expanded}
      onClick={() => setExpanded((v) => !v)}
      className={cn(
        "min-w-0 flex-1 cursor-pointer text-left hover:text-foreground",
        expanded ? "whitespace-pre-wrap break-words" : "truncate",
        className,
      )}
    >
      {expanded ? message : `${message.slice(0, max)}…`}
    </button>
  );
}
