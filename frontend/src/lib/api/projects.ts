import { req } from "./core";
import type { Project, ProjectBoard, ProjectCard, ProjectColumn, ProjectSwimlane } from "./types";

export const projectsApi = {
  // projects
  listProjects: (owner: string, name: string) =>
    req<Project[]>(`/users/${owner}/repos/${name}/projects`),
  createProject: (owner: string, name: string, projectName: string, description: string) =>
    req<Project>(`/users/${owner}/repos/${name}/projects`, {
      method: "POST",
      body: JSON.stringify({ name: projectName, description }),
    }),
  updateProject: (owner: string, name: string, id: number, fields: { name?: string; description?: string }) =>
    req<Project>(`/users/${owner}/repos/${name}/projects/${id}`, {
      method: "PATCH",
      body: JSON.stringify(fields),
    }),
  deleteProject: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/projects/${id}`, { method: "DELETE" }),

  // board
  getBoard: (owner: string, name: string, id: number) =>
    req<ProjectBoard>(`/users/${owner}/repos/${name}/projects/${id}/board`),

  // columns
  createColumn: (owner: string, name: string, id: number, columnName: string) =>
    req<ProjectColumn>(`/users/${owner}/repos/${name}/projects/${id}/columns`, {
      method: "POST",
      body: JSON.stringify({ name: columnName }),
    }),
  updateColumn: (owner: string, name: string, id: number, cid: number, fields: { name?: string; position?: number }) =>
    req<null>(`/users/${owner}/repos/${name}/projects/${id}/columns/${cid}`, {
      method: "PATCH",
      body: JSON.stringify(fields),
    }),
  deleteColumn: (owner: string, name: string, id: number, cid: number) =>
    req<null>(`/users/${owner}/repos/${name}/projects/${id}/columns/${cid}`, { method: "DELETE" }),

  // swimlanes
  createSwimlane: (owner: string, name: string, id: number, laneName: string) =>
    req<ProjectSwimlane>(`/users/${owner}/repos/${name}/projects/${id}/swimlanes`, {
      method: "POST",
      body: JSON.stringify({ name: laneName }),
    }),
  updateSwimlane: (owner: string, name: string, id: number, lid: number, fields: { name?: string; position?: number }) =>
    req<null>(`/users/${owner}/repos/${name}/projects/${id}/swimlanes/${lid}`, {
      method: "PATCH",
      body: JSON.stringify(fields),
    }),
  deleteSwimlane: (owner: string, name: string, id: number, lid: number) =>
    req<null>(`/users/${owner}/repos/${name}/projects/${id}/swimlanes/${lid}`, { method: "DELETE" }),

  // cards
  createCard: (
    owner: string,
    name: string,
    id: number,
    fields: { column_id: number; swimlane_id?: number; issue_number?: number; note?: string },
  ) =>
    req<ProjectCard>(`/users/${owner}/repos/${name}/projects/${id}/cards`, {
      method: "POST",
      body: JSON.stringify(fields),
    }),
  updateCard: (
    owner: string,
    name: string,
    id: number,
    cardId: number,
    fields: { column_id?: number; swimlane_id?: number; position?: number; note?: string },
  ) =>
    req<ProjectCard>(`/users/${owner}/repos/${name}/projects/${id}/cards/${cardId}`, {
      method: "PATCH",
      body: JSON.stringify(fields),
    }),
  deleteCard: (owner: string, name: string, id: number, cardId: number) =>
    req<null>(`/users/${owner}/repos/${name}/projects/${id}/cards/${cardId}`, { method: "DELETE" }),
};
