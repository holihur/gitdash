import { useEffect, useMemo, useRef } from "react";
import DOMPurify from "dompurify";
import { marked } from "marked";
// lib/common 只注册常用语言（~40 种），全量包体积约 4 倍于此
import hljs from "highlight.js/lib/common";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

marked.setOptions({ gfm: true, breaks: false });

/** Markdown 渲染（DOMPurify 消毒后展示；代码块由 highlight.js 高亮）。 */
export function MarkdownView({ text }: { text: string }) {
  const ref = useRef<HTMLDivElement>(null);

  const html = useMemo(() => {
    const raw = marked.parse(text ?? "", { async: false }) as string;
    return DOMPurify.sanitize(raw, { USE_PROFILES: { html: true } });
  }, [text]);

  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    root.querySelectorAll("pre code").forEach((el) => {
      if (el.textContent && el.textContent.length <= 200_000) {
        try {
          hljs.highlightElement(el as HTMLElement);
        } catch {
          /* ignore */
        }
      }
    });
    // 为 h1-h3 生成锚点 id（顺序与 extractHeadings 一致），供目录导航
    root.querySelectorAll("h1, h2, h3").forEach((el, i) => {
      if (!el.id) el.id = `md-heading-${i}`;
    });
  }, [html]);

  return (
    <div
      ref={ref}
      className="markdown-body overflow-x-auto text-sm leading-6"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}

export interface MdHeading {
  level: 1 | 2 | 3;
  text: string;
  id: string;
}

/** 从 markdown 源文本解析 #/##/### 标题（跳过代码块），id 与 MarkdownView 渲染后的锚点一一对应。 */
export function extractHeadings(text: string): MdHeading[] {
  const out: MdHeading[] = [];
  let inCode = false;
  for (const rawLine of (text ?? "").split("\n")) {
    if (/^\s*```/.test(rawLine)) inCode = !inCode;
    if (inCode) continue;
    const m = /^(#{1,3})\s+(.+?)\s*#*\s*$/.exec(rawLine.trim());
    if (!m) continue;
    const text = m[2]
      .replace(/`([^`]*)`/g, "$1")
      .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
      .replace(/[*_~]/g, "")
      .trim();
    if (text) out.push({ level: m[1].length as 1 | 2 | 3, text, id: `md-heading-${out.length}` });
  }
  return out;
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

/** 普通代码文件渲染：整段高亮后注入。 */
export function CodeText({ text, lang }: { text: string; lang?: string }) {
  const html = useMemo(() => {
    const code = text.length > 300_000 ? "" : highlight(text, lang);
    return code !== "" ? code : escapeHtml(text);
  }, [text, lang]);
  return (
    <pre className="overflow-auto text-xs leading-5">
      <code className="hljs block p-0" dangerouslySetInnerHTML={{ __html: html }} />
    </pre>
  );
}

function highlight(code: string, lang?: string): string {
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(code, { language: lang }).value;
    }
    return hljs.highlightAuto(code).value;
  } catch {
    return "";
  }
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
