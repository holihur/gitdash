import { api } from "@/lib/api";
import type { MarkdownAutocomplete, MarkdownSuggestion } from "@/components/markdown-code-editor";

/**
 * repoAutocomplete 基于仓库上下文构造 Markdown 编辑器的 @提及 / #引用 数据源。
 * 协作者接口仅 owner/admin 可见，失败时回退为仓库所有者。
 */
export function repoAutocomplete(owner: string, name: string): MarkdownAutocomplete {
  let mentions: string[] | null = null;
  let references: MarkdownSuggestion[] | null = null;

  const loadMentions = async (): Promise<string[]> => {
    if (mentions) return mentions;
    try {
      const collabs = await api.listCollabs(owner, name);
      mentions = [owner, ...collabs.map((c) => c.username)];
    } catch {
      mentions = [owner];
    }
    return mentions;
  };

  const loadReferences = async (): Promise<MarkdownSuggestion[]> => {
    if (references) return references;
    try {
      const [issues, pulls] = await Promise.all([
        api.listIssues(owner, name, 20, 0, { state: "all" }),
        api.listPulls(owner, name, undefined, 20, 0),
      ]);
      references = [
        ...issues.items.map((i) => ({
          label: `#${i.number}`,
          detail: i.title,
          insert: `${i.number} `,
        })),
        ...pulls.items.map((p) => ({
          label: `#${p.number}`,
          detail: p.title,
          insert: `${p.number} `,
        })),
      ];
    } catch {
      references = [];
    }
    return references;
  };

  return {
    mention: async (query) => {
      const q = query.toLowerCase();
      const all = await loadMentions();
      return all
        .filter((u) => u.toLowerCase().includes(q))
        .slice(0, 8)
        .map((u) => ({ label: u, detail: "mention", insert: `${u} ` }));    },
    reference: async (query) => {
      const q = query.replace(/^#/, "").toLowerCase();
      const all = await loadReferences();
      return all
        .filter((i) => i.label.toLowerCase().includes(q) || (i.detail ?? "").toLowerCase().includes(q))
        .slice(0, 8);
    },
  };
}
