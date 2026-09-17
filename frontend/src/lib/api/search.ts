import { req } from "./core";
import type { CodeSearchResponse, SearchResult } from "./types";

export const searchApi = {
  // 代码搜索（仓库内）
  searchRepo: (owner: string, name: string, q: string, ref?: string, limit = 50) => {
    const p = new URLSearchParams({ q, limit: String(limit) });
    if (ref) p.set("ref", ref);
    return req<SearchResult[]>(
      `/users/${owner}/repos/${name}/search?${p.toString()}`,
    );
  },

  // 全局代码搜索（跨仓库）
  searchCode: (
    q: string,
    opts: { repo?: string; lang?: string; path?: string; symbol?: string } = {},
  ) => {
    const p = new URLSearchParams({ q });
    if (opts.repo) p.set("repo", opts.repo);
    if (opts.lang) p.set("lang", opts.lang);
    if (opts.path) p.set("path", opts.path);
    if (opts.symbol) p.set("symbol", opts.symbol);
    return req<CodeSearchResponse>(`/search/code?${p.toString()}`);
  },
};
