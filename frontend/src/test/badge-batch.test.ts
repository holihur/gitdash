import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { invalidateBadges, requestBadges } from "@/lib/api/badges";

function badge(id: number) {
  return { id, slug: `s${id}`, label: `L${id}`, description: "", has_image: false, created_at: "" };
}

beforeEach(() => {
  invalidateBadges();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("badge batch fetcher", () => {
  it("同一 tick 内多个目标合并为一次请求", async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body)) as {
        targets: { owner: string; repo: string }[];
      };
      const items: Record<string, unknown[]> = {};
      for (const t of body.targets) items[`${t.owner}/${t.repo}`] = [badge(1)];
      return new Response(JSON.stringify({ items }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const [a, b] = await Promise.all([
      requestBadges("repo", "alice", "r1"),
      requestBadges("repo", "bob", "r2"),
    ]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(a).toHaveLength(1);
    expect(b).toHaveLength(1);
  });

  it("TTL 内命中缓存，invalidate 后重新请求", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ items: { "alice/r1": [badge(1)] } }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await requestBadges("repo", "alice", "r1");
    await requestBadges("repo", "alice", "r1");
    expect(fetchMock).toHaveBeenCalledTimes(1);

    invalidateBadges("repo", "alice", "r1");
    await requestBadges("repo", "alice", "r1");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
