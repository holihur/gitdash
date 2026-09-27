import { pageQuery, req, reqPage } from "./core";
import type { FollowState, RepoPins, UserProfile, UserSummary } from "./types";

export const usersApi = {
  /** 用户主页：资料 + 关注统计 + 可见仓库 */
  getUser: (username: string) => req<UserProfile>(`/users/${encodeURIComponent(username)}`),
  followUser: (username: string) =>
    req<FollowState>(`/users/${encodeURIComponent(username)}/follow`, { method: "POST" }),
  unfollowUser: (username: string) =>
    req<FollowState>(`/users/${encodeURIComponent(username)}/follow`, { method: "DELETE" }),
  listFollowers: (username: string, limit?: number, offset?: number) =>
    reqPage<UserSummary[]>(
      `/users/${encodeURIComponent(username)}/followers${pageQuery(limit, offset)}`,
    ),
  listFollowing: (username: string, limit?: number, offset?: number) =>
    reqPage<UserSummary[]>(
      `/users/${encodeURIComponent(username)}/following${pageQuery(limit, offset)}`,
    ),

  // 个人仓库置顶（pinned repositories）
  listPins: () => req<RepoPins>("/me/pins"),
  pinRepo: (repo: string, owner?: string) =>
    req<RepoPins>("/me/pins", {
      method: "POST",
      body: JSON.stringify({ owner: owner ?? "", repo }),
    }),
  unpinRepo: (owner: string, repo: string) =>
    req<RepoPins>(`/me/pins/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`, {
      method: "DELETE",
    }),
};
