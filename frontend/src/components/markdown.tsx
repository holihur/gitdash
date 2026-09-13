import { useEffect, useMemo, useRef, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { extractHeadings } from "@/lib/md-headings";

export { extractHeadings };
export type { MdHeading } from "@/lib/md-headings";

/**
 * Markdown 渲染（marked 解析 + DOMPurify 消毒；代码块由 highlight.js 高亮）。
 *
 * 性能：marked / dompurify / highlight.js 均为动态 import，
 * 仅在真正展示 Markdown 时才加载，highlight.js（约 150KB）更是在确有代码块时才请求，
 * 避免拖大公共 chunk 与首屏。
 */
export function MarkdownView({ text, className }: { text: string; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [html, setHtml] = useState("");

  // 解析 + 消毒：延迟到组件挂载后再拉取 marked / dompurify
  useEffect(() => {
    let alive = true;
    void (async () => {
      const [{ marked }, { default: DOMPurify }] = await Promise.all([
        import("marked"),
        import("dompurify"),
      ]);
      const raw = marked.parse(text ?? "", { async: false, gfm: true, breaks: false }) as string;
      const clean = DOMPurify.sanitize(raw, { USE_PROFILES: { html: true } });
      if (alive) setHtml(clean);
    })();
    return () => {
      alive = false;
    };
  }, [text]);

  // 标题锚点 + 代码块高亮（highlight.js 仅在存在代码块时加载）
  useEffect(() => {
    const root = ref.current;
    if (!root || !html) return;

    // 为 h1-h3 生成锚点 id（顺序与 extractHeadings 一致），供目录导航
    root.querySelectorAll("h1, h2, h3").forEach((el, i) => {
      if (!el.id) el.id = `md-heading-${i}`;
    });

    const codes = Array.from(root.querySelectorAll<HTMLElement>("pre code")).filter(
      (el) => el.textContent && el.textContent.length <= 200_000,
    );
    if (codes.length === 0) return;

    let alive = true;
    void import("highlight.js/lib/common").then(({ default: hljs }) => {
      if (!alive) return;
      codes.forEach((el) => {
        try {
          hljs.highlightElement(el);
        } catch {
          /* ignore */
        }
      });
    });
    return () => {
      alive = false;
    };
  }, [html]);

  return (
    <div
      ref={ref}
      className={cn("markdown-body overflow-x-auto text-sm leading-6", className)}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

/** Markdown + 右侧标题目录（lg 以上显示），锚点平滑滚动。 */
export function MarkdownWithToc({ text }: { text: string }) {
  const { t } = useI18n();
  const headings = useMemo(() => extractHeadings(text), [text]);
  if (headings.length === 0) return <MarkdownView text={text} />;
  const jump = (id: string) => {
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  return (
    <div className="flex gap-4">
      <div className="min-w-0 flex-1">
        <MarkdownView text={text} />
      </div>
      <nav className="hidden w-52 shrink-0 border-l pl-3 lg:block" aria-label={t("toc.title")}>
        <p className="mb-2 text-xs font-medium text-muted-foreground">{t("toc.title")}</p>
        <div className="max-h-[55vh] space-y-0.5 overflow-auto pr-1">
          {headings.map((h) => (
            <button
              key={h.id}
              className={cn(
                "block w-full truncate rounded px-1.5 py-0.5 text-left text-xs text-muted-foreground hover:bg-muted hover:text-foreground",
                h.level === 2 && "pl-4",
                h.level === 3 && "pl-7",
              )}
              onClick={() => jump(h.id)}
              title={h.text}
            >
              {h.text}
            </button>
          ))}
        </div>
      </nav>
    </div>
  );
}
