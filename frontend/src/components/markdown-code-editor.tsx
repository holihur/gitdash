import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react";
import {
  EditorView,
  drawSelection,
  highlightActiveLine,
  keymap,
  placeholder as cmPlaceholder,
} from "@codemirror/view";
import { EditorState, Prec } from "@codemirror/state";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { bracketMatching, defaultHighlightStyle, syntaxHighlighting } from "@codemirror/language";
import {
  autocompletion,
  completionKeymap,
  type Completion,
  type CompletionContext,
  type CompletionResult,
} from "@codemirror/autocomplete";
import { oneDark } from "@codemirror/theme-one-dark";

// useIsDark 读取 <html> 上的 dark class（与 ThemeProvider 应用主题的方式一致），
// 避免编辑器组件强依赖 ThemeProvider（便于单测 / 独立使用）。
function useIsDark(): boolean {
  const [dark, setDark] = useState(
    () => typeof document !== "undefined" && document.documentElement.classList.contains("dark"),
  );
  useEffect(() => {
    const el = document.documentElement;
    const observer = new MutationObserver(() => setDark(el.classList.contains("dark")));
    observer.observe(el, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, []);
  return dark;
}

/** 自动补全候选项（@提及 / #引用通用）。 */
export interface MarkdownSuggestion {
  label: string;
  detail?: string;
  /** 插入文本（不含前缀）；缺省使用 label。 */
  insert?: string;
}

/** 编辑器自动补全数据源；返回同步数组或异步 Promise。 */
export interface MarkdownAutocomplete {
  mention?: (query: string) => MarkdownSuggestion[] | Promise<MarkdownSuggestion[]>;
  reference?: (query: string) => MarkdownSuggestion[] | Promise<MarkdownSuggestion[]>;
}

/** 供工具栏调用的命令句柄。 */
export interface MarkdownEditorHandle {
  focus: () => void;
  wrapSelection: (before: string, after?: string, placeholder?: string) => void;
  toggleLinePrefix: (prefix: string) => void;
  insertBlock: (text: string) => void;
  insertLink: () => void;
  insertText: (text: string) => void;
}

interface Props {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  minHeight?: string;
  autoFocus?: boolean;
  readOnly?: boolean;
  autocomplete?: MarkdownAutocomplete;
  /** 粘贴 / 拖拽文件时的上传实现；返回可嵌入的 URL。 */
  onUpload?: (file: File) => Promise<string>;
  /** 内容区 id（与 <label htmlFor> 关联）。 */
  id?: string;
  /** 无障碍标签。 */
  ariaLabel?: string;
}

// 斜杠命令：插入常用 Markdown 片段。
const SLASH_COMMANDS: { label: string; detail: string; text: string }[] = [
  { label: "heading", detail: "Heading", text: "## " },
  { label: "bold", detail: "Bold", text: "**bold**" },
  { label: "italic", detail: "Italic", text: "*italic*" },
  { label: "code", detail: "Code block", text: "```\n\n```" },
  { label: "quote", detail: "Quote", text: "> " },
  { label: "list", detail: "Bullet list", text: "- " },
  { label: "task", detail: "Task list", text: "- [ ] " },
  { label: "table", detail: "Table", text: "| a | b |\n| --- | --- |\n|  |  |" },
  { label: "link", detail: "Link", text: "[text](url)" },
  { label: "image", detail: "Image", text: "![alt](url)" },
  { label: "rule", detail: "Divider", text: "---" },
];

function slashSource(context: CompletionContext): CompletionResult | null {
  const before = context.matchBefore(/\/[\w-]*$/);
  if (!before || (before.from === before.to && !context.explicit)) return null;
  const options: Completion[] = SLASH_COMMANDS.map((c) => ({
    label: c.label,
    detail: c.detail,
    apply: (view, _completion, from, to) => {
      view.dispatch({ changes: { from, to, insert: "" }, selection: { anchor: from } });
      insertBlockAt(view, c.text);
    },
  }));
  return { from: before.from, options, validFor: /^\/[\w-]*$/ };
}

function suggestionSource(
  prefix: string,
  get: (query: string) => MarkdownSuggestion[] | Promise<MarkdownSuggestion[]>,
): (context: CompletionContext) => Promise<CompletionResult | null> {
  return async (context) => {
    const re = prefix === "@" ? /@[\w-]*$/ : /#[\w-/]*$/;
    const before = context.matchBefore(re);
    if (!before || (before.from === before.to && !context.explicit)) return null;
    const query = before.text.slice(prefix.length);
    const items = await get(query);
    if (!items.length) return null;
    return {
      from: before.from,
      options: items.map((i) => ({
        label: i.label,
        detail: i.detail,
        apply: prefix + (i.insert ?? i.label),
      })),
      validFor: re,
    };
  };
}

/** 在光标处插入块级片段，必要时补空行。 */
function insertBlockAt(view: EditorView, text: string) {
  const { from, to } = view.state.selection.main;
  const doc = view.state.doc;
  const line = doc.lineAt(from);
  const before = from === line.from ? "" : "\n\n";
  const after = to === line.to ? "" : "\n\n";
  const insert = before + text + after;
  view.dispatch({
    changes: { from, to, insert },
    selection: { anchor: from + insert.length },
    scrollIntoView: true,
  });
  view.focus();
}

// ---- 工具栏命令实现 ----

function wrapSelection(view: EditorView, before: string, after = before, placeholder = "") {
  const { from, to } = view.state.selection.main;
  const selected = view.state.sliceDoc(from, to) || placeholder;
  view.dispatch({
    changes: { from, to, insert: before + selected + after },
    selection: { anchor: from + before.length, head: from + before.length + selected.length },
  });
  view.focus();
}

function toggleLinePrefix(view: EditorView, prefix: string) {
  const { from, to } = view.state.selection.main;
  const doc = view.state.doc;
  const start = doc.lineAt(from).number;
  const end = doc.lineAt(to).number;
  const lines = [];
  for (let n = start; n <= end; n++) lines.push(doc.line(n));
  const allHave = lines.every((l) => l.text.startsWith(prefix));
  const changes = lines.map((l) =>
    allHave
      ? { from: l.from, to: l.from + prefix.length, insert: "" }
      : { from: l.from, to: l.from, insert: prefix },
  );
  view.dispatch({ changes });
  view.focus();
}

function insertLink(view: EditorView) {
  const { from, to } = view.state.selection.main;
  const selected = view.state.sliceDoc(from, to);
  const text = selected || "text";
  const insert = `[${text}](url)`;
  view.dispatch({
    changes: { from, to, insert },
    selection: { anchor: from + 1, head: from + 1 + text.length },
  });
  view.focus();
}

function submitForm(view: EditorView): boolean {
  const form = view.dom.closest("form") as HTMLFormElement | null;
  if (!form) return false;
  if (typeof form.requestSubmit === "function") form.requestSubmit();
  else form.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
  return true;
}

/** 上传粘贴/拖拽的文件并插入 Markdown；上传中显示占位符，失败时移除。 */
async function uploadAndInsert(
  view: EditorView,
  files: File[],
  upload: (file: File) => Promise<string>,
) {
  for (const file of files) {
    const isImage = file.type.startsWith("image/");
    const placeholder = isImage ? "![uploading…]()" : "[uploading…]()";
    const { from } = view.state.selection.main;
    view.dispatch({
      changes: { from, insert: placeholder },
      selection: { anchor: from + placeholder.length },
    });
    try {
      const url = await upload(file);
      const text = isImage ? `![${file.name}](${url})` : `[${file.name}](${url})`;
      const doc = view.state.doc.toString();
      const idx = doc.indexOf(placeholder);
      if (idx >= 0) {
        view.dispatch({ changes: { from: idx, to: idx + placeholder.length, insert: text } });
      }
    } catch {
      const doc = view.state.doc.toString();
      const idx = doc.indexOf(placeholder);
      if (idx >= 0) {
        view.dispatch({ changes: { from: idx, to: idx + placeholder.length, insert: "" } });
      }
    }
  }
}

/** Markdown CodeMirror 编辑器：语法高亮、历史、自动补全、格式快捷键。 */
const MarkdownCodeEditor = forwardRef<MarkdownEditorHandle, Props>(function MarkdownCodeEditor(
  { value, onChange, placeholder, minHeight = "6rem", autoFocus, readOnly, autocomplete, onUpload, id, ariaLabel },
  ref,
) {
  const dark = useIsDark();
  const host = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const cbRef = useRef(onChange);
  cbRef.current = onChange;
  const acRef = useRef(autocomplete);
  acRef.current = autocomplete;
  const uploadRef = useRef(onUpload);
  uploadRef.current = onUpload;

  useImperativeHandle(ref, () => ({
    focus: () => viewRef.current?.focus(),
    wrapSelection: (b, a, p) => viewRef.current && wrapSelection(viewRef.current, b, a, p),
    toggleLinePrefix: (p) => viewRef.current && toggleLinePrefix(viewRef.current, p),
    insertBlock: (text) => viewRef.current && insertBlockAt(viewRef.current, text),
    insertLink: () => viewRef.current && insertLink(viewRef.current),
    insertText: (text) => {
      const view = viewRef.current;
      if (!view) return;
      const { from, to } = view.state.selection.main;
      view.dispatch({ changes: { from, to, insert: text }, selection: { anchor: from + text.length } });
      view.focus();
    },
  }), []);

  useEffect(() => {
    const el = host.current;
    if (!el) return;

    const acSources = () => {
      const ac = acRef.current;
      const sources: ((ctx: CompletionContext) => CompletionResult | null | Promise<CompletionResult | null>)[] = [slashSource];
      if (ac?.mention) sources.push(suggestionSource("@", ac.mention));
      if (ac?.reference) sources.push(suggestionSource("#", ac.reference));
      return sources;
    };

    const view = new EditorView({
      doc: value,
      extensions: [
        history(),
        drawSelection(),
        highlightActiveLine(),
        bracketMatching(),
        EditorView.lineWrapping,
        markdown({ base: markdownLanguage, addKeymap: false }),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        autocompletion({ override: acSources() }),
        EditorView.domEventHandlers({
          paste: (event, view) => {
            const files = Array.from(event.clipboardData?.files ?? []);
            if (!files.length || !uploadRef.current) return false;
            event.preventDefault();
            void uploadAndInsert(view, files, uploadRef.current);
            return true;
          },
          drop: (event, view) => {
            const files = Array.from(event.dataTransfer?.files ?? []);
            if (!files.length || !uploadRef.current) return false;
            event.preventDefault();
            void uploadAndInsert(view, files, uploadRef.current);
            return true;
          },
        }),
        dark ? oneDark : [],
        cmPlaceholder(placeholder ?? ""),
        EditorView.contentAttributes.of({
          ...(id ? { id } : {}),
          "aria-label": ariaLabel ?? placeholder ?? "Markdown",
        }),
        EditorState.readOnly.of(Boolean(readOnly)),
        Prec.highest(
          keymap.of([
            { key: "Mod-b", run: (v) => (wrapSelection(v, "**", "**", "bold"), true) },
            { key: "Mod-i", run: (v) => (wrapSelection(v, "*", "*", "italic"), true) },
            { key: "Mod-e", run: (v) => (wrapSelection(v, "`", "`", "code"), true) },
            { key: "Mod-k", run: (v) => (insertLink(v), true) },
            { key: "Mod-Enter", run: (v) => submitForm(v) },
          ]),
        ),
        keymap.of([...defaultKeymap, ...historyKeymap, ...completionKeymap, indentWithTab]),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) cbRef.current(u.state.doc.toString());
        }),
        EditorView.theme({
          "&": { minHeight, fontSize: "13px" },
          ".cm-scroller": { fontFamily: "var(--font-mono), monospace", lineHeight: "1.6" },
          ".cm-content": { padding: "0.6rem 0.75rem" },
          "&.cm-focused": { outline: "none" },
          ".cm-tooltip-autocomplete": {
            border: "1px solid var(--border)",
            borderRadius: "var(--radius)",
            background: "var(--popover)",
            color: "var(--popover-foreground)",
          },
        }),
      ],
      parent: el,
    });
    viewRef.current = view;
    if (autoFocus) view.focus();
    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dark, readOnly, minHeight]);

  // 外部 value 变化：增量同步
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (value !== current) {
      view.dispatch({ changes: { from: 0, to: current.length, insert: value } });
    }
  }, [value]);

  return <div ref={host} className="max-h-[60vh] overflow-auto" />;
});

export default MarkdownCodeEditor;
