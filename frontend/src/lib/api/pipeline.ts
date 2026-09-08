import { req } from "./core";
import type { PipelineRun, Runner } from "./types";

export const pipelineApi = {
  // pipeline（CI）
  getPipeline: (owner: string, name: string) =>
    req<{ enabled: boolean; file: string }>(`/users/${owner}/repos/${name}/pipeline`),
  setPipeline: (owner: string, name: string, enabled: boolean) =>
    req<{ enabled: boolean }>(`/users/${owner}/repos/${name}/pipeline`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  listPipelineRuns: (owner: string, name: string) =>
    req<PipelineRun[]>(`/users/${owner}/repos/${name}/pipeline/runs?limit=50`),
  getPipelineRun: (owner: string, name: string, id: number) =>
    req<PipelineRun>(`/users/${owner}/repos/${name}/pipeline/runs/${id}`),
  triggerPipelineRun: (owner: string, name: string, ref?: string) =>
    req<PipelineRun>(`/users/${owner}/repos/${name}/pipeline/runs`, {
      method: "POST",
      body: JSON.stringify(ref ? { ref } : {}),
    }),
  cancelPipelineRun: (owner: string, name: string, id: number) =>
    req<{ cancelled: boolean }>(`/users/${owner}/repos/${name}/pipeline/runs/${id}/cancel`, {
      method: "POST",
    }),


  // runners（自托管 CI agent）
  listRunners: () => req<Runner[]>("/runners"),
  createRunnerToken: (scope: "user" | "org", org?: string) =>
    req<{ token: string; scope: string; expires_at: string }>("/runners/registration-token", {
      method: "POST",
      body: JSON.stringify({ scope, org }),
    }),
  deleteRunner: (name: string) => req<null>(`/runners/${encodeURIComponent(name)}`, { method: "DELETE" }),

};
