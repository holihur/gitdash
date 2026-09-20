import { req } from "./core";
import type { BatchImportResult, Connection, RemoteRepo } from "./types";

export const connectionsApi = {
  listConnections: () => req<Connection[]>("/connections"),
  disconnect: (provider: string) => req<null>(`/connections/${provider}`, { method: "DELETE" }),
  listRemoteRepos: (provider: string) => req<RemoteRepo[]>(`/connections/${provider}/repos`),
  batchImport: (input: {
    provider: string;
    namespace?: string;
    private?: boolean;
    repos: string[];
  }) =>
    req<BatchImportResult>("/imports/batch", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  /** 授权跳转 URL（浏览器整页导航，携带 httpOnly 会话 cookie）。 */
  connectUrl: (provider: string) => `/api/connections/${provider}/start`,
};
