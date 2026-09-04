import { useEffect, useRef } from "react";
import { EditorView, basicSetup } from "codemirror";
import { EditorState, Extension } from "@codemirror/state";
import { oneDark } from "@codemirror/theme-one-dark";
import { javascript } from "@codemirror/lang-javascript";
import { python } from "@codemirror/lang-python";
import { markdown } from "@codemirror/lang-markdown";
import { json } from "@codemirror/lang-json";
import { css } from "@codemirror/lang-css";
import { html } from "@codemirror/lang-html";
import { java } from "@codemirror/lang-java";
import { cpp } from "@codemirror/lang-cpp";
import { rust } from "@codemirror/lang-rust";
import { sql } from "@codemirror/lang-sql";
import { php } from "@codemirror/lang-php";
import { useTheme } from "@/lib/theme";

const LANGUAGE_BY_EXT: Record<string, Extension> = {
  ts: javascript({ typescript: true }),
  tsx: javascript({ jsx: true, typescript: true }),
  js: javascript(),
  jsx: javascript({ jsx: true }),
  mjs: javascript(),
  cjs: javascript(),
  py: python(),
  md: markdown(),
  markdown: markdown(),
  mkd: markdown(),
  json: json(),
  css: css(),
  scss: css(),
  less: css(),
  html: html(),
  htm: html(),
  xml: html(),
  vue: html(),
  java: java(),
  c: cpp(),
  h: cpp(),
  cpp: cpp(),
  hpp: cpp(),
  cc: cpp(),
  rust: rust(),
  rs: rust(),
  sql: sql(),
  php: php(),
};

export function languageForPath(path: string): Extension | undefined {
  const name = path.split("/").pop() ?? "";
  const ext = (name.split(".").pop() ?? "").toLowerCase();
  if (name.toLowerCase() === "dockerfile") return undefined;
  return LANGUAGE_BY_EXT[ext];
}

interface Props {
  value: string;
  path?: string;
  readOnly?: boolean;
  className?: string;
  onDocChange?: (value: string) => void;
}

/** CodeMirror 6 封装：明暗主题 + 语法高亮（语言由 path 自动推断）。 */
export default function CodeMirrorEditor({
  value,
  path,
  readOnly = false,
  className,
  onDocChange,
}: Props) {
  const { resolved } = useTheme();
  const dark = resolved === "dark";
  const host = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const cbRef = useRef(onDocChange);
  cbRef.current = onDocChange;

  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const view = new EditorView({
      doc: value,
      extensions: [
        basicSetup,
        (path ? languageForPath(path) : undefined) ?? [],
        EditorView.theme({
          "&": { height: "100%", fontSize: "13px" },
          ".cm-scroller": { fontFamily: "var(--font-mono), monospace", lineHeight: "1.6" },
          ".cm-content": { padding: "0.7rem 0" },
          "&.cm-focused": { outline: "none" },
        }),
        dark ? oneDark : [],
        readOnly ? EditorState.readOnly.of(true) : [],
        EditorView.updateListener.of((u) => {
          if (u.docChanged) cbRef.current?.(u.state.doc.toString());
        }),
      ],
      parent: el,
    });
    viewRef.current = view;
    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dark, readOnly, value, path]);

  return <div ref={host} className={className} style={{ overflow: "hidden" }} />;
}
