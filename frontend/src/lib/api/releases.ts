import { req, sendForm } from "./core";
import type { Release } from "./types";

export const releasesApi = {
  // releases
  listReleases: (owner: string, name: string) =>
    req<Release[]>(`/users/${owner}/repos/${name}/releases`),
  createRelease: (owner: string, name: string, tagName: string, title?: string, body?: string) =>
    req<Release>(`/users/${owner}/repos/${name}/releases`, {
      method: "POST",
      body: JSON.stringify({ tag_name: tagName, name: title, body }),
    }),
  deleteRelease: (owner: string, name: string, tag: string) =>
    req<null>(`/users/${owner}/repos/${name}/releases/${encodeURIComponent(tag)}`, {
      method: "DELETE",
    }),
  uploadReleaseAsset: (owner: string, name: string, tag: string, file: File) => {
    const form = new FormData();
    form.append("file", file);
    return sendForm(
      `/users/${owner}/repos/${name}/releases/${encodeURIComponent(tag)}/assets`,
      form,
    );
  },
  deleteReleaseAsset: (owner: string, name: string, tag: string, filename: string) =>
    req<null>(
      `/users/${owner}/repos/${name}/releases/${encodeURIComponent(tag)}/assets/${encodeURIComponent(filename)}`,
      { method: "DELETE" },
    ),
  releaseAssetUrl: (owner: string, name: string, tag: string, filename: string) =>
    `/api/users/${owner}/repos/${name}/releases/${encodeURIComponent(tag)}/assets/${encodeURIComponent(filename)}`,

};
