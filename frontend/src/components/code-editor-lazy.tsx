import { lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";

// 按需加载真正的编辑器（codemirror 核心 + 主题）：浏览/查看代码时不拉取
const Editor = lazy(() => import("@/components/code-editor"));

interface Props {
  value: string;
  path?: string;
  readOnly?: boolean;
  className?: string;
  onDocChange?: (value: string) => void;
}

export default function CodeMirrorEditor(props: Props) {
  return (
    <Suspense fallback={<Skeleton className={props.className ?? "h-40 w-full"} />}>
      <Editor {...props} />
    </Suspense>
  );
}
