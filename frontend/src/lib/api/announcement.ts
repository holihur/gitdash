import { req } from "./core";
import type { Announcement } from "./types";

export const announcementApi = {
  /** 全站通知（公开接口，登录与否都可读取）。 */
  announcement: () => req<Announcement>("/announcement"),
};
