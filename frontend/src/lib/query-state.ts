import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

type Patch = Record<string, string | number | null | undefined>;

/**
 * 把列表页状态(页码/页大小/筛选/搜索词)同步进 URL 查询参数。
 * get(key, fallback) 读取，set({key: value}) 写入；null/undefined 删除该键。
 * 默认 replace 历史，避免筛选输入刷爆后退栈；分页可传 { push: true }。
 */
export function useQueryState() {
  const [searchParams, setSearchParams] = useSearchParams();

  const get = useCallback(
    (key: string, fallback: string): string => searchParams.get(key) ?? fallback,
    [searchParams],
  );

  const getNum = useCallback(
    (key: string, fallback: number): number => {
      const n = Number(searchParams.get(key));
      return Number.isFinite(n) && n > 0 ? n : fallback;
    },
    [searchParams],
  );

  const set = useCallback(
    (patch: Patch, opts?: { push?: boolean }) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          for (const [k, v] of Object.entries(patch)) {
            if (v === null || v === undefined) next.delete(k);
            else next.set(k, String(v));
          }
          return next;
        },
        { replace: !opts?.push },
      );
    },
    [setSearchParams],
  );

  return { get, getNum, set };
}
