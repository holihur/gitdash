import { lazy, Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";

// 按需加载真正的编辑器（codemirror 核心 + 主题）：浏览/查看代码时不拉取
const Editor = lazy(() => import("@/components/code-editor"));

/** preloadEditor 预加载编辑器 chunk（含 CodeMirror 核心与主题），在代码页挂载后空闲时调用。 */
export function preloadEditor(): void {
  void import("@/components/code-editor");
}

/** preloadFile 预加载打开某文件所需的编辑器与语言包（文件树/列表 hover 时调用）。 */
export async function preloadFile(path: string): Promise<void> {
  try {
    const m = await import("@/components/code-editor");
    await m.preloadLanguage(path);
  } catch {
    /* 预加载失败不影响后续正常加载 */
  }
}

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
