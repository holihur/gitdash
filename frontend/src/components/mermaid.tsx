import { useEffect, useRef } from "react";
import { cn } from "@/lib/utils";

/**
 * 懒加载 mermaid 并渲染给定节点（第一次调用时才下载 mermaid，体积较大）。
 * 仅当页面确有图表时才请求，避免拖大公共 chunk。
 */
export async function renderMermaid(nodes: HTMLElement[]): Promise<void> {
  if (nodes.length === 0) return;
  const { default: mermaid } = await import("mermaid");
  const dark = document.documentElement.classList.contains("dark");
  mermaid.initialize({
    startOnLoad: false,
    // strict：对图表内的 HTML/脚本做安全处理，防止 XSS
    securityLevel: "strict",
    theme: dark ? "dark" : "default",
    fontFamily: "inherit",
    flowchart: { htmlLabels: false, curve: "basis" },
  });
  await mermaid.run({ nodes });
}

/** 把 mermaid 源码渲染为图表（源码以 textContent 写入，不解析 HTML）。 */
export function MermaidDiagram({ chart, className }: { chart: string; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.removeAttribute("data-processed");
    el.textContent = chart;
    renderMermaid([el]).catch(() => {
      /* 非法图表：mermaid 会在容器内渲染错误提示 */
    });
  }, [chart]);
  return <div ref={ref} className={cn("mermaid", className)} />;
}
