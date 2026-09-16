// diff 解析与类型：从 unified patch 中提取文件/行号，供 SplitDiff 与行内评论定位复用。

export interface DiffFileInfo {
  path: string;
  status: "A" | "M" | "D";
  insertions: number;
  deletions: number;
}

export interface ParsedLine {
  raw: string;
  cls: string;
  /** 命中行所属文件（diff --git 头解析） */
  file: string;
  old?: number;
  new?: number;
}

export interface FileChunk {
  path: string;
  lines: ParsedLine[];
}

/** 行内评论定位键 */
export const commentKey = (file: string, side: "old" | "new", line: number) =>
  `${file}|${side}|${line}`;

export interface CommentTarget {
  file: string;
  line: number;
  side: "old" | "new";
}

/** SplitDiff 行渲染回调的上下文 */
export interface DiffLineContext {
  line: ParsedLine;
  file: string;
  chunk: number;
  index: number;
  id: string;
}

export const statusTextClass = (status: DiffFileInfo["status"]) =>
  status === "A"
    ? "text-green-600 dark:text-green-400"
    : status === "D"
      ? "text-red-600 dark:text-red-400"
      : "text-yellow-600 dark:text-yellow-400";

/** 解析 unified patch：为每行标注文件路径与 old/new 行号（文件头与 index 行无行号） */
export function parsePatch(patch: string): ParsedLine[] {
  const out: ParsedLine[] = [];
  let file = "";
  let oldLine = 0;
  let newLine = 0;
  for (const raw of patch.split("\n")) {
    let cls = "";
    if (raw.startsWith("diff --git")) {
      const m = /^diff --git a\/(.*) b\/(.*)$/.exec(raw);
      file = m ? m[1] : "";
      cls = "text-muted-foreground";
    } else if (raw.startsWith("index ")) {
      cls = "text-muted-foreground";
    } else if (raw.startsWith("@@")) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(raw);
      if (m) {
        oldLine = Number(m[1]);
        newLine = Number(m[2]);
      }
      cls = "text-blue-600 dark:text-blue-400";
    } else if (raw.startsWith("+") && !raw.startsWith("+++")) {
      cls = "bg-green-500/15 text-green-700 dark:text-green-400";
      out.push({ raw, cls, file, new: newLine });
      newLine += 1;
      continue;
    } else if (raw.startsWith("-") && !raw.startsWith("---")) {
      cls = "bg-red-500/15 text-red-700 dark:text-red-400";
      out.push({ raw, cls, file, old: oldLine });
      oldLine += 1;
      continue;
    } else if (raw.startsWith("---") || raw.startsWith("+++")) {
      cls = "text-muted-foreground";
    } else if (raw !== "") {
      out.push({ raw, cls, file, old: oldLine, new: newLine });
      oldLine += 1;
      newLine += 1;
      continue;
    }
    out.push({ raw, cls, file });
  }
  return out;
}

/** 按文件对 patch 行分组；文件头之前的行并入首个文件 */
export function groupLines(lines: ParsedLine[]): FileChunk[] {
  const chunks: FileChunk[] = [];
  const pending: ParsedLine[] = [];
  let current: FileChunk | null = null;

  for (const line of lines) {
    if (line.file) {
      if (!current || current.path !== line.file) {
        current = { path: line.file, lines: [] };
        if (pending.length) {
          current.lines.push(...pending);
          pending.length = 0;
        }
        chunks.push(current);
      }
      current.lines.push(line);
    } else if (current) {
      current.lines.push(line);
    } else {
      pending.push(line);
    }
  }

  if (pending.length) chunks.push({ path: "", lines: pending });
  return chunks;
}
