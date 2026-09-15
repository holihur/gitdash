import { req } from "./core";
import type { IncomingWebhook, Webhook, WebhookDelivery } from "./types";

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

  // incoming webhook（入站：token 创建 issue）
  getIncomingWebhook: (owner: string, name: string) =>
    req<IncomingWebhook>(`/users/${owner}/repos/${name}/incoming-webhook`),
  setIncomingWebhook: (owner: string, name: string) =>
    req<IncomingWebhook>(`/users/${owner}/repos/${name}/incoming-webhook`, { method: "POST" }),
  deleteIncomingWebhook: (owner: string, name: string) =>
    req<null>(`/users/${owner}/repos/${name}/incoming-webhook`, { method: "DELETE" }),

};
