import type { Commit } from "@/lib/api";

/** 提交图布局：每行对应一个提交，含泳道与连线信息（纯函数，便于测试）。 */
export interface CommitGraphRow {
  /** 本提交所在泳道 */
  lane: number;
  /** 泳道总数（用于计算列宽，所有行统一） */
  lanes: number;
  /** 从本行顶部直达底部的竖线泳道（其它提交的血缘线） */
  through: number[];
  /** 从顶部汇入本提交的泳道（合并汇聚） */
  mergesIn: number[];
  /** 每个父提交对应泳道（与 commit.parents 一一对应） */
  parentLanes: number[];
  /** 是否有来自顶部的线接入本提交（本提交是上方提交的父节点） */
  fromTop: boolean;
}

/**
 * 计算提交图布局。输入按新→旧排序（git log 顺序），每项需带 parents。
 * 使用经典的「泳道」算法：本提交的首个父提交沿用其泳道，其余父提交分配新泳道；
 * 同一提交被多条泳道等待时（合并汇聚）保留最左泳道，其余汇入。
 */
export function buildCommitGraph(commits: Commit[]): CommitGraphRow[] {
  const lanes: (string | null)[] = [];
  const rows: CommitGraphRow[] = [];

  const findLane = (sha: string) => lanes.findIndex((s) => s === sha);
  const allocLane = () => {
    const free = lanes.indexOf(null);
    if (free >= 0) return free;
    lanes.push(null);
    return lanes.length - 1;
  };

  for (const c of commits) {
    const before = lanes.slice();
    const existing: number[] = [];
    for (let i = 0; i < lanes.length; i++) {
      if (lanes[i] === c.sha) existing.push(i);
    }
    const lane = existing.length > 0 ? existing[0] : allocLane();
    // 汇聚：保留最左泳道，其余置空（连线由 mergesIn 绘制）
    for (const dup of existing) {
      if (dup !== lane) lanes[dup] = null;
    }

    const parents = c.parents ?? [];
    lanes[lane] = parents[0] ?? null;
    const parentLanes: number[] = [];
    for (let k = 0; k < parents.length; k++) {
      if (k === 0) {
        parentLanes.push(lane);
        continue;
      }
      let pl = findLane(parents[k]);
      if (pl < 0) {
        pl = allocLane();
        lanes[pl] = parents[k];
      }
      parentLanes.push(pl);
    }

    const total = Math.max(before.length, lanes.length);
    const through: number[] = [];
    for (let i = 0; i < total; i++) {
      if (i === lane) continue;
      if (before[i] && lanes[i]) through.push(i);
    }
    rows.push({
      lane,
      lanes: total,
      through,
      mergesIn: existing.filter((i) => i !== lane),
      parentLanes,
      fromTop: before[lane] === c.sha,
    });

    while (lanes.length && lanes[lanes.length - 1] === null) lanes.pop();
  }

  const maxLanes = rows.reduce((m, r) => Math.max(m, r.lanes), 1);
  for (const r of rows) r.lanes = maxLanes;
  return rows;
}

/** 泳道配色（循环使用，深浅模式下均可见）。 */
export const LANE_COLORS = [
  "#2563eb", // blue
  "#16a34a", // green
  "#d97706", // amber
  "#dc2626", // red
  "#7c3aed", // violet
  "#0891b2", // cyan
  "#db2777", // pink
  "#65a30d", // lime
];

export function laneColor(lane: number): string {
  return LANE_COLORS[lane % LANE_COLORS.length];
}
