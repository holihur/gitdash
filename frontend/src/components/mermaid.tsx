import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

let seq = 0;

/** 懒加载 mermaid 并完成初始化（第一次调用时才下载 mermaid，体积较大）。 */
async function loadMermaid() {
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
  return mermaid;
}

/**
 * 离屏把 Mermaid 源码渲染为 SVG 字符串。
 *
 * 不再把源码写进容器再交给 `mermaid.run()`：mermaid 是异步按需加载的大 chunk，
 * 在它加载/渲染完成前，写进 DOM 的源码会被当成普通文本显示出来
 * （浏览器还会折叠换行，看起来就是一行乱码 `flowchart TD nstart(...)`）。
 */
export async function renderMermaidSvg(chart: string): Promise<string> {
  const mermaid = await loadMermaid();
  const id = `mermaid-${Date.now().toString(36)}-${seq++}`;
  const { svg } = await mermaid.render(id, chart);
  return svg;
}

/**
 * 把图表渲染进给定容器，`charts[i]` 渲染到 `nodes[i]`。
 * 渲染完成前容器保持空白，避免闪现未渲染的源码。
 */
export async function renderMermaid(nodes: HTMLElement[], charts: string[]): Promise<void> {
  await Promise.all(
    nodes.map(async (node, i) => {
      node.innerHTML = await renderMermaidSvg(charts[i] ?? "");
    }),
  );
}

/** 把 mermaid 源码渲染为图表（离屏渲染后注入，避免加载期间显示源码）。 */
export function MermaidDiagram({ chart, className }: { chart: string; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");

  useEffect(() => {
    let alive = true;
    const el = ref.current;
    if (!el) return;
    setState("loading");
    el.innerHTML = "";
    renderMermaidSvg(chart)
      .then((svg) => {
        if (!alive || !ref.current) return;
        ref.current.innerHTML = svg;
        setState("ready");
      })
      .catch(() => {
        if (alive) setState("error");
      });
    return () => {
      alive = false;
    };
  }, [chart]);

  return (
    <div className={cn("space-y-2", className)}>
      {/* 渲染中显示占位骨架，而不是未渲染的源码 */}
      {state === "loading" && <div className="h-32 w-full animate-pulse rounded-md bg-muted/40" />}
      <div ref={ref} className="mermaid" hidden={state !== "ready"} />
      {state === "error" && (
        <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 font-mono text-xs leading-relaxed">
          {chart}
        </pre>
      )}
    </div>
  );
}
