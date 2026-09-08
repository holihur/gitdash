import { req } from "./core";
import type { Webhook, WebhookDelivery } from "./types";

export const webhooksApi = {
  // webhooks
  listWebhooks: (owner: string, name: string) =>
    req<Webhook[]>(`/users/${owner}/repos/${name}/webhooks`),
  createWebhook: (owner: string, name: string, url: string, secret?: string) =>
    req<Webhook>(`/users/${owner}/repos/${name}/webhooks`, {
      method: "POST",
      body: JSON.stringify({ url, secret: secret ?? "" }),
    }),
  deleteWebhook: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/webhooks/${id}`, { method: "DELETE" }),
  listWebhookDeliveries: (owner: string, name: string, id: number) =>
    req<WebhookDelivery[]>(`/users/${owner}/repos/${name}/webhooks/${id}/deliveries?limit=20`),

};
