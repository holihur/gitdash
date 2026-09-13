/** 文件大纲（outline）抽取：Markdown 标题 + 常见语言的顶层符号（尽力而为的启发式）。 */

import { extractHeadings } from "@/lib/md-headings";

export interface OutlineItem {
  /** 展示文本 */
  text: string;
  /** 层级（0 = 顶层） */
  level: number;
  /** 粗粒度符号类型 */
  kind: "heading" | "function" | "type" | "variable" | "section";
  /** 1-based 行号（代码文件） */
  line?: number;
  /** 锚点 id（Markdown，与 MarkdownView 渲染出的 id 一致） */
  id?: string;
}

function isMarkdown(path: string): boolean {
  const base = path.split("/").pop() ?? "";
  const lower = base.toLowerCase();
  return /^readme(\.(md|markdown|txt))?$/.test(lower) || /\.(md|markdown)$/.test(lower);
}

function extOf(path: string): string {
  const base = path.split("/").pop() ?? "";
  const idx = base.lastIndexOf(".");
  return idx >= 0 ? base.slice(idx + 1).toLowerCase() : "";
}

function indentLevel(raw: string): number {
  const leading = raw.match(/^\s*/)?.[0].length ?? 0;
  if (leading === 0) return 0;
  return leading >= 8 ? 2 : 1;
}

/** Go：func（含方法）与顶层 type/struct/interface/const/var。 */
function goOutline(lines: string[]): OutlineItem[] {
  const out: OutlineItem[] = [];
  const funcRe = /^\s*func\s+(?:\(\s*\w+\s+\*?[A-Za-z0-9_.]+\s*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)/;
  const typeRe = /^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)/;
  const varRe = /^\s*(?:const|var)\s+(?:\(|\s*([A-Za-z_][A-Za-z0-9_]*))/;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    let m = funcRe.exec(line);
    if (m) {
      out.push({ text: m[1], level: indentLevel(line), kind: "function", line: i + 1 });
      continue;
    }
    m = typeRe.exec(line);
    if (m) {
      out.push({ text: m[1], level: indentLevel(line), kind: "type", line: i + 1 });
      continue;
    }
    m = varRe.exec(line);
    if (m && m[1]) {
      out.push({ text: m[1], level: indentLevel(line), kind: "variable", line: i + 1 });
    }
  }
  return out;
}

/** Python：class / 顶层 def（方法按缩进层级）。 */
function pythonOutline(lines: string[]): OutlineItem[] {
  const out: OutlineItem[] = [];
  const re = /^\s*(class|def)\s+([A-Za-z_][A-Za-z0-9_]*)/;
  for (let i = 0; i < lines.length; i++) {
    const m = re.exec(lines[i]);
    if (!m) continue;
    out.push({
      text: m[2],
      level: indentLevel(lines[i]),
      kind: m[1] === "class" ? "type" : "function",
      line: i + 1,
    });
  }
  return out;
}

/** JavaScript / TypeScript：function、class、箭头函数常量、export。 */
function jsOutline(lines: string[]): OutlineItem[] {
  const out: OutlineItem[] = [];
  const funcRe = /^\s*(?:export\s+)?(?:async\s+)?function\s+\*?\s*([A-Za-z_$][A-Za-z0-9_$]*)/;
  const classRe = /^\s*(?:export\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)/;
  const arrowRe = /^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>/;
  const methodRe = /^\s{2,}(?:static\s+|async\s+)*(?:get\s+|set\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)\s*\{/;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    let m = funcRe.exec(line);
    if (m) {
      out.push({ text: m[1], level: indentLevel(line), kind: "function", line: i + 1 });
      continue;
    }
    m = classRe.exec(line);
    if (m) {
      out.push({ text: m[1], level: indentLevel(line), kind: "type", line: i + 1 });
      continue;
    }
    m = arrowRe.exec(line);
    if (m) {
      out.push({ text: m[1], level: indentLevel(line), kind: "variable", line: i + 1 });
      continue;
    }
    m = methodRe.exec(line);
    if (m) {
      out.push({ text: m[1], level: 1, kind: "function", line: i + 1 });
    }
  }
  return out;
}

/** Rust：fn / struct / enum / trait / impl。 */
function rustOutline(lines: string[]): OutlineItem[] {
  const out: OutlineItem[] = [];
  const re = /^\s*(?:pub\s+)?(fn|struct|enum|trait|impl|mod|const|static)\s+([A-Za-z_][A-Za-z0-9_]*)/;
  for (let i = 0; i < lines.length; i++) {
    const m = re.exec(lines[i]);
    if (!m) continue;
    out.push({
      text: m[2],
      level: indentLevel(lines[i]),
      kind: m[1] === "fn" ? "function" : m[1] === "const" || m[1] === "static" ? "variable" : "type",
      line: i + 1,
    });
  }
  return out;
}

/** C / C++ / Java / C#：函数签名与类型声明（启发式）。 */
function cLikeOutline(lines: string[]): OutlineItem[] {
  const out: OutlineItem[] = [];
  // 类型声明
  const typeRe = /^\s*(?:public\s+|private\s+|protected\s+|internal\s+|static\s+|abstract\s+|sealed\s+)*(?:class|struct|enum|interface|record)\s+([A-Za-z_][A-Za-z0-9_]*)/;
  // 函数：返回类型 + 名称( 或 构造函数；避免 if/for/while/switch/catch
  const funcRe = /^\s*(?:public\s+|private\s+|protected\s+|internal\s+|static\s+|virtual\s+|override\s+|final\s+|abstract\s+|async\s+|synchronized\s+)*(?:[A-Za-z_:][A-Za-z0-9_:<>,[\]*&\s]*?)\s+([A-Za-z_~][A-Za-z0-9_]*)\s*\([^;{}]*\)\s*(?:const\s*)?(?:\{|$)/;
  const keyword = /^\s*(if|for|while|switch|catch|return|new|sizeof|typeof)\b/;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (keyword.test(line)) continue;
    const tm = typeRe.exec(line);
    if (tm) {
      out.push({ text: tm[1], level: indentLevel(line), kind: "type", line: i + 1 });
      continue;
    }
    const fm = funcRe.exec(line);
    if (fm && !/^(if|for|while|switch|catch|return|else|do|new|case|sizeof|typedef|using)$/.test(fm[1])) {
      out.push({ text: fm[1], level: indentLevel(line), kind: "function", line: i + 1 });
    }
  }
  return out;
}

/** Shell：函数定义。 */
function shOutline(lines: string[]): OutlineItem[] {
  const out: OutlineItem[] = [];
  const re = /^\s*(?:function\s+)?([A-Za-z_][A-Za-z0-9_-]*)\s*\(\)\s*\{/;
  for (let i = 0; i < lines.length; i++) {
    const m = re.exec(lines[i]);
    if (m) out.push({ text: m[1], level: 0, kind: "function", line: i + 1 });
  }
  return out;
}

/** YAML：顶层 key（忽略列表项与文档分隔符）。 */
function yamlOutline(lines: string[]): OutlineItem[] {
  const out: OutlineItem[] = [];
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith(" ") || line.startsWith("\t")) continue;
    if (/^(---|\.\.\.|#)/.test(line) || line.trim() === "") continue;
    const m = /^([^:#\s][^:]*):/.exec(line);
    if (m) out.push({ text: m[1].trim(), level: 0, kind: "section", line: i + 1 });
  }
  return out;
}

/** 从文件内容抽取大纲：Markdown 用标题，代码用语言启发式。 */
export function extractOutline(path: string, content: string): OutlineItem[] {
  if (!content) return [];
  if (isMarkdown(path)) {
    return extractHeadings(content).map((h) => ({
      text: h.text,
      level: h.level - 1,
      kind: "heading" as const,
      id: h.id,
    }));
  }
  const lines = content.split("\n");
  if (lines.length > 20000) return []; // 超大文件不解析大纲
  const ext = extOf(path);
  switch (ext) {
    case "go":
      return goOutline(lines);
    case "py":
    case "pyw":
      return pythonOutline(lines);
    case "js":
    case "jsx":
    case "mjs":
    case "cjs":
    case "ts":
    case "tsx":
      return jsOutline(lines);
    case "rs":
      return rustOutline(lines);
    case "c":
    case "h":
    case "cpp":
    case "hpp":
    case "cc":
    case "hh":
    case "cxx":
    case "java":
    case "cs":
      return cLikeOutline(lines);
    case "sh":
    case "bash":
    case "zsh":
      return shOutline(lines);
    case "yml":
    case "yaml":
      return yamlOutline(lines);
    default:
      return [];
  }
}
