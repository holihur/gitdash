/**
 * 轻量 Markdown 标题解析：不依赖 marked / dompurify / highlight.js。
 * 供目录（TOC）与文件大纲使用，避免为解析标题而加载整个 Markdown 渲染栈。
 */

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
