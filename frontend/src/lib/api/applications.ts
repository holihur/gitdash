import { req } from "./core";
import type { CreatedOAuthApp, OAuthApp, OAuthAuthorization } from "./types";

export const applicationsApi = {
  listApps: () => req<OAuthApp[]>("/applications"),
  createApp: (input: {
    name: string;
    homepage: string;
    description: string;
    callback_url: string;
  }) =>
    req<CreatedOAuthApp>("/applications", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  deleteApp: (id: number) => req<null>(`/applications/${id}`, { method: "DELETE" }),
  resetSecret: (id: number) =>
    req<{ client_secret: string }>(`/applications/${id}/reset_secret`, {
      method: "POST",
    }),
  listAuthorizations: () => req<OAuthAuthorization[]>("/applications/authorizations"),
  revokeAuthorization: (id: number) =>
    req<null>(`/applications/authorizations/${id}`, { method: "DELETE" }),
};
