import { req } from "./core";
import type { Badge } from "./types";

export type BadgeTargetKind = "user" | "repo" | "org";

/** 徽章图标地址（无图返回 undefined）；带版本号破缓存。 */
export function badgeImageUrl(b: Badge): string | undefined {
  if (!b.has_image) return undefined;
  const v = b.image_updated_at ? `?v=${encodeURIComponent(b.image_updated_at)}` : "";
  return `/api/badges/${b.id}/image${v}`;
}

function targetQuery(kind: BadgeTargetKind, owner: string, repo?: string): string {
  const p = new URLSearchParams({ kind, owner });
  if (repo) p.set("repo", repo);
  return p.toString();
}

export interface OwnedBadges {
  granted: Badge[];
  displayed: number[];
  max: number;
}

// ---- 批量取展示徽章（列表卡片 / 搜索结果一次拉取，避免逐条请求）----

function getBadgesBatchRaw(kind: BadgeTargetKind, targets: { owner: string; repo: string }[]) {
  return req<{ items: Record<string, Badge[]> }>("/badges/batch", {
    method: "POST",
    body: JSON.stringify({ kind, targets }),
  });
}

type TargetReq = { kind: BadgeTargetKind; owner: string; repo?: string };

const keyOf = (kind: string, owner: string, repo?: string) => `${kind}\x1f${owner}\x1f${repo ?? ""}`;
const serverKey = (owner: string, repo?: string) => `${owner}/${repo ?? ""}`;

// 展示结果缓存：带 TTL，避免一次会话内反复请求；写操作后可主动失效。
const BADGE_CACHE_TTL_MS = 60_000;
const cache = new Map<string, { at: number; badges: Badge[] }>();
let queue: TargetReq[] = [];
let waiters: { key: string; resolve: (b: Badge[]) => void }[] = [];
let scheduled = false;

/** 失效徽章缓存（不带参数则清空）；设置挂载后调用。 */
export function invalidateBadges(kind?: BadgeTargetKind, owner?: string, repo?: string): void {
  if (kind === undefined) {
    cache.clear();
    return;
  }
  cache.delete(keyOf(kind, owner ?? "", repo));
}

async function flushBatch() {
  const batch = queue;
  queue = [];
  const ws = waiters;
  waiters = [];
  scheduled = false;

  const byKind = new Map<BadgeTargetKind, TargetReq[]>();
  const seen = new Set<string>();
  for (const t of batch) {
    const k = keyOf(t.kind, t.owner, t.repo);
    if (seen.has(k)) continue;
    seen.add(k);
    const arr = byKind.get(t.kind) ?? [];
    arr.push(t);
    byKind.set(t.kind, arr);
  }

  const lookup = new Map<string, Badge[]>();
  await Promise.all(
    [...byKind.entries()].map(async ([kind, ts]) => {
      try {
        const res = await getBadgesBatchRaw(
          kind,
          ts.map((t) => ({ owner: t.owner, repo: t.repo ?? "" })),
        );
        for (const t of ts) {
          lookup.set(keyOf(kind, t.owner, t.repo), res.items[serverKey(t.owner, t.repo)] ?? []);
        }
      } catch {
        for (const t of ts) lookup.set(keyOf(kind, t.owner, t.repo), []);
      }
    }),
  );

  for (const w of ws) {
    const b = lookup.get(w.key) ?? [];
    cache.set(w.key, { at: Date.now(), badges: b });
    w.resolve(b);
  }
}

/** 请求某目标展示的徽章；同一 tick 内多个请求会合并成一次批量接口。 */
export function requestBadges(kind: BadgeTargetKind, owner: string, repo?: string): Promise<Badge[]> {
  const k = keyOf(kind, owner, repo);
  const cached = cache.get(k);
  if (cached && Date.now() - cached.at < BADGE_CACHE_TTL_MS) {
    return Promise.resolve(cached.badges);
  }
  return new Promise((resolve) => {
    queue.push({ kind, owner, repo });
    waiters.push({ key: k, resolve });
    if (!scheduled) {
      scheduled = true;
      queueMicrotask(flushBatch);
    }
  });
}

export const badgesApi = {
  /** 某目标公开展示的徽章（自动批量化）。 */
  getBadges: (kind: BadgeTargetKind, owner: string, repo?: string) => requestBadges(kind, owner, repo),
  /** 原始批量接口。 */
  getBadgesBatch: getBadgesBatchRaw,
  /** 某目标已获得 / 已挂出的徽章（需写权限）。 */
  getOwned: (kind: BadgeTargetKind, owner: string, repo?: string) =>
    req<OwnedBadges>(`/badges/owned?${targetQuery(kind, owner, repo)}`),
  /** 设置目标挂出的徽章（最多 max 个）。 */
  setDisplay: async (kind: BadgeTargetKind, owner: string, repo: string | undefined, badgeIds: number[]) => {
    const res = await req<{ displayed: Badge[] }>("/badges/display", {
      method: "PUT",
      body: JSON.stringify({ kind, owner, repo: repo ?? "", badge_ids: badgeIds }),
    });
    // 写操作后失效对应缓存，下次拉取展示最新结果。
    invalidateBadges(kind, owner, repo);
    return res;
  },
};
