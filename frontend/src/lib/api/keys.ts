import { req } from "./core";
import type { CreatedPAT, GPGKey, PAT, SSHKey } from "./types";

export const keysApi = {
  // ssh keys
  listKeys: () => req<SSHKey[]>("/keys"),
  createKey: (name: string, publicKey: string) =>
    req<SSHKey>("/keys", { method: "POST", body: JSON.stringify({ name, public_key: publicKey }) }),
  deleteKey: (id: number) => req<null>(`/keys/${id}`, { method: "DELETE" }),


  // gpg keys（提交签名验证）
  listGPGKeys: () => req<GPGKey[]>("/gpg"),
  addGPGKey: (armor: string) =>
    req<GPGKey>("/gpg", { method: "POST", body: JSON.stringify({ armor }) }),
  deleteGPGKey: (id: number) => req<null>(`/gpg/${id}`, { method: "DELETE" }),


  // personal access tokens
  listPATs: () => req<PAT[]>("/tokens"),
  createPAT: (name: string, scopes: string[], cidrs: string[], expiresAt: string) =>
    req<CreatedPAT>("/tokens", {
      method: "POST",
      body: JSON.stringify({ name, scopes, cidrs, expires_at: expiresAt }),
    }),
  deletePAT: (id: number) => req<null>(`/tokens/${id}`, { method: "DELETE" }),

};
