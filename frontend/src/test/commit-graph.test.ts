import { describe, expect, it } from "vitest";
import { buildCommitGraph } from "@/lib/commit-graph";
import type { Commit } from "@/lib/api";

function c(sha: string, parents: string[]): Commit {
  return { sha, author: "alice", date: "2026-01-01T00:00:00Z", message: sha, parents };
}

describe("buildCommitGraph", () => {
  it("linear history keeps a single lane", () => {
    const rows = buildCommitGraph([c("c", ["b"]), c("b", ["a"]), c("a", [])]);
    expect(rows.map((r) => r.lane)).toEqual([0, 0, 0]);
    expect(rows[0].fromTop).toBe(false);
    expect(rows[1].fromTop).toBe(true);
    expect(rows[1].parentLanes).toEqual([0]);
    expect(rows.every((r) => r.lanes === 1)).toBe(true);
  });

  it("merge gives the second parent a new lane and converges", () => {
    const rows = buildCommitGraph([
      c("m", ["a", "b"]),
      c("a", ["base"]),
      c("b", ["base"]),
      c("base", []),
    ]);
    expect(rows[0].lane).toBe(0);
    expect(rows[0].parentLanes).toEqual([0, 1]);
    expect(rows[1].lane).toBe(0);
    expect(rows[2].lane).toBe(1);
    // base is awaited by both lanes: leftmost wins, the other merges in.
    expect(rows[3].lane).toBe(0);
    expect(rows[3].mergesIn).toEqual([1]);
    expect(rows.every((r) => r.lanes === 2)).toBe(true);
  });

  it("handles a root commit without parents", () => {
    const rows = buildCommitGraph([c("x", [])]);
    expect(rows[0].lane).toBe(0);
    expect(rows[0].parentLanes).toEqual([]);
  });
});
