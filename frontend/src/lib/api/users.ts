import { req } from "./core";
import type { FollowState, UserProfile, UserSummary } from "./types";

export const usersApi = {
  /** 用户主页：资料 + 关注统计 + 可见仓库 */
  getUser: (username: string) => req<UserProfile>(`/users/${encodeURIComponent(username)}`),
  followUser: (username: string) =>
    req<FollowState>(`/users/${encodeURIComponent(username)}/follow`, { method: "POST" }),
  unfollowUser: (username: string) =>
    req<FollowState>(`/users/${encodeURIComponent(username)}/follow`, { method: "DELETE" }),
  listFollowers: (username: string) =>
    req<UserSummary[]>(`/users/${encodeURIComponent(username)}/followers`),
  listFollowing: (username: string) =>
    req<UserSummary[]>(`/users/${encodeURIComponent(username)}/following`),
};
