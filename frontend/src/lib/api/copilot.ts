import { req } from "./core";
import type { ByokKey, CopilotSession } from "./types";

export const copilotApi = {
  // byok（bring your own key：用户自带 LLM 密钥）
  listByok: () => req<ByokKey[]>("/me/byok"),
  createByok: (body: {
    name: string;
    provider: string;
    api_key: string;
    base_url?: string;
    model?: string;
  }) =>
    req<ByokKey>("/me/byok", { method: "POST", body: JSON.stringify(body) }),
  updateByok: (
    id: number,
    body: { name: string; provider: string; api_key?: string; base_url?: string; model?: string },
  ) =>
    req<ByokKey>(`/me/byok/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteByok: (id: number) => req<{ deleted: boolean }>(`/me/byok/${id}`, { method: "DELETE" }),

  // copilot 会话（每个会话一个独立 Docker 容器）
  listCopilots: (owner: string, repo: string) =>
    req<CopilotSession[]>(`/users/${owner}/repos/${repo}/copilots`),
  createCopilot: (
    owner: string,
    repo: string,
    body: { byok_id: number; image?: string; prompt?: string; command?: string },
  ) =>
    req<CopilotSession>(`/users/${owner}/repos/${repo}/copilots`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  getCopilot: (owner: string, repo: string, id: number) =>
    req<CopilotSession>(`/users/${owner}/repos/${repo}/copilots/${id}`),
  startCopilot: (owner: string, repo: string, id: number) =>
    req<CopilotSession>(`/users/${owner}/repos/${repo}/copilots/${id}/start`, { method: "POST" }),
  stopCopilot: (owner: string, repo: string, id: number) =>
    req<CopilotSession>(`/users/${owner}/repos/${repo}/copilots/${id}/stop`, { method: "POST" }),
  deleteCopilot: (owner: string, repo: string, id: number) =>
    req<{ deleted: boolean }>(`/users/${owner}/repos/${repo}/copilots/${id}`, { method: "DELETE" }),
};
