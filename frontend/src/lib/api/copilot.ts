import { req } from "./core";
import type { ByokKey, CopilotMessage, CopilotSession } from "./types";

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
  /** 测试 BYOK 连接：api_key 留空且带 id 时回退到已保存密钥。 */
  testByok: (body: {
    id?: number;
    provider: string;
    api_key?: string;
    base_url?: string;
    model?: string;
  }) =>
    req<{ ok: boolean; base_url?: string; model?: string; error?: string }>("/me/byok/test", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  // copilot 会话（每个会话一个 agent 运行时，双向聊天 + 自动提交推送）
  listCopilots: (owner: string, repo: string) =>
    req<CopilotSession[]>(`/users/${owner}/repos/${repo}/copilots`),
  createCopilot: (owner: string, repo: string, body: { byok_id: number; prompt?: string; issue_number?: number }) =>
    req<CopilotSession>(`/users/${owner}/repos/${repo}/copilots`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  getCopilot: (owner: string, repo: string, id: number) =>
    req<CopilotSession>(`/users/${owner}/repos/${repo}/copilots/${id}`),
  copilotMessages: (owner: string, repo: string, id: number) =>
    req<CopilotMessage[]>(`/users/${owner}/repos/${repo}/copilots/${id}/messages`),
  stopCopilot: (owner: string, repo: string, id: number) =>
    req<CopilotSession>(`/users/${owner}/repos/${repo}/copilots/${id}/stop`, { method: "POST" }),
  deleteCopilot: (owner: string, repo: string, id: number) =>
    req<{ deleted: boolean }>(`/users/${owner}/repos/${repo}/copilots/${id}`, { method: "DELETE" }),

  /** 双向聊天 WebSocket 地址（同源 Cookie 自动携带）。 */
  copilotChatUrl: (owner: string, repo: string, id: number) => {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${window.location.host}/api/users/${owner}/repos/${repo}/copilots/${id}/chat`;
  },
};
