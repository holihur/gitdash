import { useEffect, useRef } from "react";
import { EditorView, basicSetup } from "codemirror";
import { Compartment, EditorState, type Extension } from "@codemirror/state";
import { oneDark } from "@codemirror/theme-one-dark";
import { useTheme } from "@/lib/theme";

// 语言包按需动态加载：仅在编辑对应扩展名的文件时才拉取对应 chunk
async function loadLanguage(path: string): Promise<Extension | undefined> {
  const name = path.split("/").pop() ?? "";
  if (name.toLowerCase() === "dockerfile") return undefined;
  const ext = (name.split(".").pop() ?? "").toLowerCase();
  switch (ext) {
    case "ts":
      return (await import("@codemirror/lang-javascript")).javascript({ typescript: true });
    case "tsx":
      return (await import("@codemirror/lang-javascript")).javascript({ jsx: true, typescript: true });
    case "jsx":
      return (await import("@codemirror/lang-javascript")).javascript({ jsx: true });
    case "js":
    case "mjs":
    case "cjs":
      return (await import("@codemirror/lang-javascript")).javascript();
    case "py":
      return (await import("@codemirror/lang-python")).python();
    case "md":
    case "markdown":
    case "mkd":
      return (await import("@codemirror/lang-markdown")).markdown();
    case "json":
      return (await import("@codemirror/lang-json")).json();
    case "css":
    case "scss":
    case "less":
      return (await import("@codemirror/lang-css")).css();
    case "html":
    case "htm":
    case "xml":
    case "vue":
      return (await import("@codemirror/lang-html")).html();
    case "java":
      return (await import("@codemirror/lang-java")).java();
    case "c":
    case "h":
    case "cpp":
    case "hpp":
    case "cc":
      return (await import("@codemirror/lang-cpp")).cpp();
    case "rust":
    case "rs":
      return (await import("@codemirror/lang-rust")).rust();
    case "sql":
      return (await import("@codemirror/lang-sql")).sql();
    case "php":
      return (await import("@codemirror/lang-php")).php();
    default:
      return undefined;
  }
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
    // 语言用 Compartment 挂载：编辑器先立即可用，语言包异步加载后再注入
    const langCompartment = new Compartment();
    const view = new EditorView({
      doc: value,
      extensions: [
        basicSetup,
        langCompartment.of([]),
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
    let cancelled = false;
    if (path) {
      loadLanguage(path).then((lang) => {
        if (!cancelled) view.dispatch({ effects: langCompartment.reconfigure(lang ?? []) });
      });
    }
    return () => {
      cancelled = true;
      view.destroy();
      viewRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dark, readOnly, value, path]);

  return <div ref={host} className={className} style={{ overflow: "hidden" }} />;
}
