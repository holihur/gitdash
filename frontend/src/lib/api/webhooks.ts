import { req } from "./core";
import type { IncomingWebhookCreated, IncomingWebhookList, Webhook, WebhookDelivery } from "./types";

export const webhooksApi = {
  // webhooks
  listWebhooks: (owner: string, name: string) =>
    req<Webhook[]>(`/users/${owner}/repos/${name}/webhooks`),
  createWebhook: (owner: string, name: string, url: string, secret?: string, events?: string[]) =>
    req<Webhook>(`/users/${owner}/repos/${name}/webhooks`, {
      method: "POST",
      body: JSON.stringify({ url, secret: secret ?? "", events: events ?? [] }),
    }),
  listWebhookEvents: () => req<string[]>("/webhook-events"),
  deleteWebhook: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/webhooks/${id}`, { method: "DELETE" }),
  listWebhookDeliveries: (owner: string, name: string, id: number) =>
    req<WebhookDelivery[]>(`/users/${owner}/repos/${name}/webhooks/${id}/deliveries?limit=20`),

  // incoming webhooks（入站：token 创建 issue，支持多个）
  listIncomingWebhooks: (owner: string, name: string) =>
    req<IncomingWebhookList>(`/users/${owner}/repos/${name}/incoming-webhooks`),
  createIncomingWebhook: (owner: string, name: string, hookName?: string) =>
    req<IncomingWebhookCreated>(`/users/${owner}/repos/${name}/incoming-webhooks`, {
      method: "POST",
      body: JSON.stringify({ name: hookName ?? "" }),
    }),
  deleteIncomingWebhook: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/incoming-webhooks/${id}`, { method: "DELETE" }),
  setIncomingWebhookEnabled: (owner: string, name: string, id: number, enabled: boolean) =>
    req<{ enabled: boolean }>(`/users/${owner}/repos/${name}/incoming-webhooks/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }),
};
