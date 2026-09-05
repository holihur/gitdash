import { useEffect, useMemo, useRef } from "react";
import DOMPurify from "dompurify";
import { marked } from "marked";
// lib/common 只注册常用语言（~40 种），全量包体积约 4 倍于此
import hljs from "highlight.js/lib/common";

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
  }, [html]);

  return (
    <div
      ref={ref}
      className="markdown-body overflow-x-auto text-sm leading-6"
      dangerouslySetInnerHTML={{ __html: html }}
    />
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
