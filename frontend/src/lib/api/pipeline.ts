import { req } from "./core";
import type { ArtifactFile, PipelineGraph, PipelineRun, RepoEnvVar, RepoSecret, Runner } from "./types";

export const pipelineApi = {
  // pipeline（CI）
  getPipeline: (owner: string, name: string) =>
    req<{ enabled: boolean; file: string; files: string[] }>(`/users/${owner}/repos/${name}/pipeline`),
  getPipelineGraph: (owner: string, name: string, ref?: string, file?: string) => {
    const params = new URLSearchParams();
    if (ref) params.set("ref", ref);
    if (file) params.set("file", file);
    const qs = params.toString();
    return req<PipelineGraph>(`/users/${owner}/repos/${name}/pipeline/graph${qs ? `?${qs}` : ""}`);
  },
  setPipeline: (owner: string, name: string, enabled: boolean) =>
    req<{ enabled: boolean }>(`/users/${owner}/repos/${name}/pipeline`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  listPipelineRuns: (owner: string, name: string) =>
    req<PipelineRun[]>(`/users/${owner}/repos/${name}/pipeline/runs?limit=50`),
  getPipelineRun: (owner: string, name: string, id: number) =>
    req<PipelineRun>(`/users/${owner}/repos/${name}/pipeline/runs/${id}`),
  triggerPipelineRun: (owner: string, name: string, body?: { ref?: string; sha?: string; file?: string; delay?: string } | string) =>
    req<{ runs: PipelineRun[] }>(`/users/${owner}/repos/${name}/pipeline/runs`, {
      method: "POST",
      body: JSON.stringify(typeof body === "string" ? { ref: body } : (body ?? {})),
    }),
  rerunPipelineRun: (owner: string, name: string, id: number) =>
    req<PipelineRun>(`/users/${owner}/repos/${name}/pipeline/runs/${id}/rerun`, {
      method: "POST",
    }),
  dispatchPipelineRun: (
    owner: string,
    name: string,
    body?: { ref?: string; sha?: string; file?: string; inputs?: Record<string, string> },
  ) =>
    req<{ runs: PipelineRun[] }>(`/users/${owner}/repos/${name}/pipeline/dispatch`, {
      method: "POST",
      body: JSON.stringify(body ?? {}),
    }),
  cancelPipelineRun: (owner: string, name: string, id: number) =>
    req<{ cancelled: boolean }>(`/users/${owner}/repos/${name}/pipeline/runs/${id}/cancel`, {
      method: "POST",
    }),

  // 运行产物：列表 + 下载 URL（同源 Cookie 鉴权，供 <a download> 使用）
  listRunArtifacts: (owner: string, name: string, id: number) =>
    req<ArtifactFile[]>(`/users/${owner}/repos/${name}/pipeline/runs/${id}/artifacts`),
  runArtifactsUrl: (owner: string, name: string, id: number) =>
    `/api/users/${owner}/repos/${name}/pipeline/runs/${id}/artifacts/download`,

  // 仓库级流水线环境变量（仅 owner）
  listRepoEnvVars: (owner: string, name: string) =>
    req<RepoEnvVar[]>(`/users/${owner}/repos/${name}/env`),
  setRepoEnvVar: (owner: string, name: string, key: string, value: string) =>
    req<RepoEnvVar[]>(`/users/${owner}/repos/${name}/env`, {
      method: "PUT",
      body: JSON.stringify({ key, value }),
    }),
  deleteRepoEnvVar: (owner: string, name: string, key: string) =>
    req<{ deleted: boolean }>(`/users/${owner}/repos/${name}/env/${encodeURIComponent(key)}`, {
      method: "DELETE",
    }),

  // 仓库 CI secrets（仅 owner，值加密存储且永不回传）
  listRepoSecrets: (owner: string, name: string) =>
    req<RepoSecret[]>(`/users/${owner}/repos/${name}/secrets`),
  setRepoSecret: (owner: string, name: string, secretName: string, value: string) =>
    req<RepoSecret[]>(`/users/${owner}/repos/${name}/secrets`, {
      method: "PUT",
      body: JSON.stringify({ name: secretName, value }),
    }),
  deleteRepoSecret: (owner: string, name: string, secretName: string) =>
    req<{ deleted: boolean }>(
      `/users/${owner}/repos/${name}/secrets/${encodeURIComponent(secretName)}`,
      { method: "DELETE" },
    ),


  // runners（自托管 CI agent）
  listRunners: () => req<Runner[]>("/runners"),
  createRunnerToken: (scope: "user" | "org", org?: string) =>
    req<{ token: string; scope: string; expires_at: string }>("/runners/registration-token", {
      method: "POST",
      body: JSON.stringify({ scope, org }),
    }),
  deleteRunner: (name: string) => req<null>(`/runners/${encodeURIComponent(name)}`, { method: "DELETE" }),

};
