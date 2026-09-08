import { req } from "./core";
import type { SearchResult } from "./types";

export const searchApi = {
  // 代码搜索
  searchRepo: (owner: string, name: string, q: string, ref?: string, limit = 50) => {
    const p = new URLSearchParams({ q, limit: String(limit) });
    if (ref) p.set("ref", ref);
    return req<SearchResult[]>(
      `/users/${owner}/repos/${name}/search?${p.toString()}`,
    );
  },

};
