import { pageQuery, req, reqPage } from "./core";
import type { Notification, Repo } from "./types";

export const inboxApi = {
  // watch & inbox
  listWatched: () => req<Repo[]>("/watched"),
  watch: (owner: string, name: string) =>
    req<{ watching: boolean; watchers: number }>(`/users/${owner}/repos/${name}/watch`, {
      method: "PUT",
    }),
  unwatch: (owner: string, name: string) =>
    req<{ watching: boolean; watchers: number }>(`/users/${owner}/repos/${name}/watch`, {
      method: "DELETE",
    }),
  inbox: (limit?: number, offset?: number) =>
    reqPage<Notification[]>(`/inbox${pageQuery(limit, offset)}`),
  inboxUnread: () => req<{ count: number }>("/inbox/unread"),
  inboxRead: (id: number) =>
    req<{ ok: boolean }>(`/inbox/read/${id}`, { method: "POST" }),
  inboxReadAll: () => req<{ ok: boolean }>("/inbox/read", { method: "POST" }),
  inboxDelete: (id: number) => req<null>(`/inbox/${id}`, { method: "DELETE" }),
  forkRepo: (owner: string, name: string, opts?: { name?: string; namespace?: string }) =>
    req<Repo>(`/users/${owner}/repos/${name}/fork`, {
      method: "POST",
      body: JSON.stringify(opts ?? {}),
    }),
  importRepo: (opts: {
    url: string;
    name?: string;
    namespace?: string;
    private?: boolean;
    private_key?: string;
  }) =>
    req<Repo>("/imports", {
      method: "POST",
      body: JSON.stringify(opts),
    }),

};
