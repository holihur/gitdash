import { useEffect, useMemo, useRef, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { cn, copyText } from "@/lib/utils";
import { extractHeadings } from "@/lib/md-headings";
import { renderMermaid } from "@/components/mermaid";

export { extractHeadings };
export type { MdHeading } from "@/lib/md-headings";

/**
 * Markdown 渲染（marked 解析 + DOMPurify 消毒；代码块由 highlight.js 高亮）。
 *
 * 性能：marked / dompurify / highlight.js 均为动态 import，
 * 仅在真正展示 Markdown 时才加载，highlight.js（约 150KB）更是在确有代码块时才请求，
 * 避免拖大公共 chunk 与首屏。
 */
const COPY_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
const CHECK_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';

// addCopyButtons 把每个 <pre> 包进 .code-block 容器，并在右上角注入复制按钮
// （按钮放在滚动容器之外，横向滚动代码时保持固定）。
function addCopyButtons(
  root: HTMLElement,
  t: (key: string) => string,
  cleanups: Array<() => void>,
) {
  const pres = Array.from(root.querySelectorAll<HTMLElement>("pre")).filter(
    (pre) => pre.querySelector("code") && !pre.closest(".mermaid") && !pre.closest(".code-block"),
  );
  for (const pre of pres) {
    const wrapper = document.createElement("div");
    wrapper.className = "code-block";
    pre.replaceWith(wrapper);
    wrapper.appendChild(pre);

    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "code-copy-btn";
    btn.title = t("common.copy");
    btn.setAttribute("aria-label", t("common.copy"));
    btn.innerHTML = COPY_ICON;

    let timer: number | undefined;
    const onClick = () => {
      const code = pre.querySelector("code")?.textContent ?? "";
      copyText(code)
        .then(() => {
          btn.innerHTML = CHECK_ICON;
          btn.classList.add("copied");
          btn.title = t("common.copied");
          window.clearTimeout(timer);
          timer = window.setTimeout(() => {
            btn.innerHTML = COPY_ICON;
            btn.classList.remove("copied");
            btn.title = t("common.copy");
          }, 1500);
        })
        .catch(() => {
          /* ignore */
        });
    };
    btn.addEventListener("click", onClick);
    wrapper.appendChild(btn);

    cleanups.push(() => {
      window.clearTimeout(timer);
      btn.removeEventListener("click", onClick);
    });
  }
}

export function MarkdownView({ text, className }: { text: string; className?: string }) {
  const { t } = useI18n();
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
      // 显式收紧白名单：仅允许 HTML profile，禁掉样式/表单/嵌入类标签与内联 style；
      // 默认已拦截 javascript: 等危险 URI，这里不再自定义 ALLOWED_URI_REGEXP，
      // 以免破坏相对链接、锚点与 mailto。
      const clean = DOMPurify.sanitize(raw, {
        USE_PROFILES: { html: true },
        FORBID_TAGS: ["style", "form", "iframe", "object", "embed", "link", "meta", "base"],
        FORBID_ATTR: ["style"],
      });
      if (alive) setHtml(clean);
    })();
    return () => {
      alive = false;
    };
  }, [text]);

  // 标题锚点 + Mermaid 图表 + 代码块高亮（highlight.js / mermaid 均按需加载）
  useEffect(() => {
    const root = ref.current;
    if (!root || !html) return;

    // 为 h1-h3 生成锚点 id（顺序与 extractHeadings 一致），供目录导航
    root.querySelectorAll("h1, h2, h3").forEach((el, i) => {
      if (!el.id) el.id = `md-heading-${i}`;
    });

    // Mermaid：把 ```mermaid 代码块换成图表容器；仅确有图表时才加载 mermaid（体积较大）。
    const mermaidCodes = Array.from(
      root.querySelectorAll<HTMLElement>("pre > code.language-mermaid"),
    );
    const mermaidNodes: HTMLElement[] = [];
    const mermaidCharts: string[] = [];
    for (const code of mermaidCodes) {
      const pre = code.parentElement;
      if (!pre) continue;
      const div = document.createElement("div");
      div.className = "mermaid";
      pre.replaceWith(div);
      mermaidNodes.push(div);
      // 源码先留在内存里离屏渲染，不写进 DOM，避免加载期间闪现未渲染的源码
      mermaidCharts.push(code.textContent ?? "");
    }

    let alive = true;
    if (mermaidNodes.length > 0) {
      void renderMermaid(mermaidNodes, mermaidCharts).catch(() => {
        /* 非法图表：渲染失败时容器保持为空 */
      });
    }

    // 其余代码块高亮（mermaid 块已被换成 div，不在其中）
    const codes = Array.from(root.querySelectorAll<HTMLElement>("pre code")).filter(
      (el) =>
        el.textContent &&
        el.textContent.length <= 200_000 &&
        !el.classList.contains("language-mermaid"),
    );

    const cleanups: Array<() => void> = [];
    const withCopyButtons = () => addCopyButtons(root, t, cleanups);

    if (codes.length === 0) {
      withCopyButtons();
    } else {
      void import("highlight.js/lib/common").then(({ default: hljs }) => {
        if (!alive) return;
        codes.forEach((el) => {
          try {
            hljs.highlightElement(el);
          } catch {
            /* ignore */
          }
        });
        withCopyButtons();
      });
    }
    return () => {
      alive = false;
      cleanups.forEach((fn) => fn());
    };
  }, [html, t]);

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
