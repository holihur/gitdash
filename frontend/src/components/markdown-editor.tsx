import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import {
  Bold,
  Code,
  Columns2,
  Eye,
  Heading2,
  Image as ImageIcon,
  Italic,
  Link as LinkIcon,
  List,
  ListChecks,
  ListOrdered,
  Minus,
  Pencil,
  Quote,
  SquareCode,
  Strikethrough,
  Table,
} from "lucide-react";

import { api } from "@/lib/api";
import type { RepoLinkContext, RepoLinkTarget } from "@/lib/md-links";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { Textarea } from "@/components/ui/textarea";
import { MarkdownView } from "@/components/markdown";
import type { MarkdownAutocomplete, MarkdownEditorHandle } from "@/components/markdown-code-editor";

// CodeMirror（含语法高亮依赖）体积较大：编辑器整体懒加载，首屏只保留纯文本回退。
const LazyMarkdownCodeEditor = lazy(() => import("@/components/markdown-code-editor"));

type Mode = "write" | "preview" | "split";

interface ToolbarAction {
  key: string;
  icon: typeof Bold;
  title: string;
  run: (editor: MarkdownEditorHandle) => void;
}

/** Markdown 编辑器：CodeMirror 语法高亮 + 工具栏 + 快捷键 + 预览/分屏 + 草稿 + 上传。 */
export function MarkdownEditor({
  id,
  value,
  onChange,
  placeholder,
  rows = 4,
  maxLength,
  autoFocus,
  className,
  repo,
  onOpenRepoLink,
  draftKey,
  autocomplete,
  upload,
}: {
  id?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  rows?: number;
  maxLength?: number;
  autoFocus?: boolean;
  className?: string;
  /** 预览时的仓库上下文（仓库内相对链接会改写为代码浏览路由）。 */
  repo?: RepoLinkContext;
  onOpenRepoLink?: (target: RepoLinkTarget) => void;
  /** 提供后按维度自动保存 / 恢复草稿（localStorage）。 */
  draftKey?: string;
  /** @提及 / #引用 自动补全数据源。 */
  autocomplete?: MarkdownAutocomplete;
  /** 自定义上传实现；缺省使用 /api/uploads。 */
  upload?: (file: File) => Promise<string>;
}) {
  const { t } = useI18n();
  const [mode, setMode] = useState<Mode>("write");
  const [editor, setEditor] = useState<MarkdownEditorHandle | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const minHeight = `${Math.max(rows, 3) * 1.5}rem`;

  const uploader = upload ?? (async (file: File) => (await api.uploadFile(file)).url);

  // 草稿恢复：仅在内容为空时用本地草稿回填（避免覆盖已有内容）。
  const storageKey = draftKey ? `gitdash-md-draft:${draftKey}` : "";
  useEffect(() => {
    if (!storageKey || value) return;
    try {
      const saved = window.localStorage.getItem(storageKey);
      if (saved) onChange(saved);
    } catch {
      /* localStorage 不可用（隐私模式）时忽略 */
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [storageKey]);

  // 草稿自动保存（防抖）：内容清空时移除草稿。
  useEffect(() => {
    if (!storageKey) return;
    const timer = window.setTimeout(() => {
      try {
        if (value.trim()) window.localStorage.setItem(storageKey, value);
        else window.localStorage.removeItem(storageKey);
      } catch {
        /* ignore */
      }
    }, 400);
    return () => window.clearTimeout(timer);
  }, [value, storageKey]);

  const run = (action: (target: MarkdownEditorHandle) => void) => {
    if (!editor) return;
    action(editor);
    editor.focus();
  };

  const toolbar: ToolbarAction[] = [
    { key: "bold", icon: Bold, title: t("markdown.bold"), run: (e) => e.wrapSelection("**", "**", "bold") },
    { key: "italic", icon: Italic, title: t("markdown.italic"), run: (e) => e.wrapSelection("*", "*", "italic") },
    { key: "strike", icon: Strikethrough, title: t("markdown.strike"), run: (e) => e.wrapSelection("~~", "~~", "text") },
    { key: "heading", icon: Heading2, title: t("markdown.heading"), run: (e) => e.toggleLinePrefix("## ") },
    { key: "quote", icon: Quote, title: t("markdown.quote"), run: (e) => e.toggleLinePrefix("> ") },
    { key: "list", icon: List, title: t("markdown.bulletList"), run: (e) => e.toggleLinePrefix("- ") },
    { key: "ordered", icon: ListOrdered, title: t("markdown.orderedList"), run: (e) => e.toggleLinePrefix("1. ") },
    { key: "task", icon: ListChecks, title: t("markdown.taskList"), run: (e) => e.toggleLinePrefix("- [ ] ") },
    { key: "code", icon: Code, title: t("markdown.inlineCode"), run: (e) => e.wrapSelection("`", "`", "code") },
    { key: "codeblock", icon: SquareCode, title: t("markdown.codeBlock"), run: (e) => e.insertBlock("```\n\n```") },
    { key: "link", icon: LinkIcon, title: t("markdown.link"), run: (e) => e.insertLink() },
    { key: "table", icon: Table, title: t("markdown.table"), run: (e) => e.insertBlock("| a | b |\n| --- | --- |\n|  |  |") },
    { key: "rule", icon: Minus, title: t("markdown.divider"), run: (e) => e.insertBlock("---") },
  ];

  const pickImage = async (file: File) => {
    try {
      const url = await uploader(file);
      run((e) => e.insertText(`![${file.name}](${url})`));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("markdown.uploadFailed"));
    }
  };

  const modes: { key: Mode; label: string; icon: typeof Pencil }[] = [
    { key: "write", label: t("common.write"), icon: Pencil },
    { key: "preview", label: t("common.preview"), icon: Eye },
    { key: "split", label: t("markdown.split"), icon: Columns2 },
  ];

  const editorNode = (
    <Editor
      id={id}
      value={value}
      onChange={onChange}
      placeholder={placeholder}
      minHeight={minHeight}
      autoFocus={autoFocus}
      autocomplete={autocomplete}
      onUpload={uploader}
      onInstance={setEditor}
    />
  );

  const previewNode = value.trim() ? (
    <MarkdownView text={value} repo={repo} onOpenRepoLink={onOpenRepoLink} />
  ) : (
    <p className="text-sm text-muted-foreground">{t("common.nothingToPreview")}</p>
  );

  return (
    <div className={cn("space-y-1.5", className)}>
      <div className="flex flex-wrap items-center gap-1">
        <div className="flex items-center gap-0.5" aria-label={t("markdown.modeLabel")}>
          {modes.map((m) => (
            <button
              key={m.key}
              type="button"
              aria-pressed={mode === m.key}
              onClick={() => setMode(m.key)}
              className={cn(
                "flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium",
                mode === m.key
                  ? "bg-muted text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              <m.icon className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">{m.label}</span>
            </button>
          ))}
        </div>

        {mode !== "preview" && (
          <div className="flex items-center gap-0.5" role="toolbar" aria-label={t("markdown.toolbar")}>
            {toolbar.map((b) => (
              <button
                key={b.key}
                type="button"
                title={b.title}
                aria-label={b.title}
                onClick={() => run(b.run)}
                className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
              >
                <b.icon className="h-3.5 w-3.5" />
              </button>
            ))}
            <button
              type="button"
              title={t("markdown.uploadImage")}
              aria-label={t("markdown.uploadImage")}
              onClick={() => fileInputRef.current?.click()}
              className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <ImageIcon className="h-3.5 w-3.5" />
            </button>
          </div>
        )}

        <span className="ml-auto text-[11px] text-muted-foreground">
          {t("common.markdownSupported")}
          {maxLength ? ` · ${value.length}/${maxLength}` : ""}
        </span>
      </div>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/png,image/jpeg,image/gif,image/webp,application/pdf,text/plain"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (file) void pickImage(file);
        }}
      />

      {mode === "preview" ? (
        <div className="min-h-24 rounded-md border bg-background px-3 py-2">{previewNode}</div>
      ) : mode === "split" ? (
        <div className="grid gap-2 lg:grid-cols-2">
          <div className="rounded-md border">{editorNode}</div>
          <div className="max-h-[60vh] overflow-auto rounded-md border bg-background px-3 py-2">
            {previewNode}
          </div>
        </div>
      ) : (
        <div className="rounded-md border">{editorNode}</div>
      )}
    </div>
  );
}

/** 懒加载包装：编辑器 chunk 未就绪时用纯文本 textarea 兜底，用户可立即输入。 */
function Editor({
  onInstance,
  ...props
}: {
  onInstance: (instance: MarkdownEditorHandle | null) => void;
  id?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  minHeight?: string;
  autoFocus?: boolean;
  autocomplete?: MarkdownAutocomplete;
  onUpload?: (file: File) => Promise<string>;
}) {
  return (
    <Suspense
      fallback={
        <Textarea
          id={props.id}
          value={props.value}
          placeholder={props.placeholder}
          autoFocus={props.autoFocus}
          onChange={(e) => props.onChange(e.target.value)}
          style={{ minHeight: props.minHeight }}
          className="rounded-none border-0 focus-visible:ring-0"
        />
      }
    >
      <LazyMarkdownCodeEditor ref={onInstance} {...props} />
    </Suspense>
  );
}
