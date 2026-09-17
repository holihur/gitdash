import { req, reqPage } from "./core";
import type { BranchProtection, CodeownersStatus, Issue, IssueComment, Label, MergeGate, Milestone, PullDiff, PullRequest, PullReview, PullState, ReviewState } from "./types";

export const issuesApi = {
  // issues
  listIssues: (
    owner: string,
    name: string,
    limit?: number,
    offset?: number,
    filters?: { q?: string; state?: string },
  ) => {
    const params = new URLSearchParams();
    if (limit) params.set("limit", String(limit));
    if (offset) params.set("offset", String(offset));
    if (filters?.q) params.set("q", filters.q);
    if (filters?.state) params.set("state", filters.state);
    const qs = params.toString();
    return reqPage<Issue[]>(`/users/${owner}/repos/${name}/issues${qs ? `?${qs}` : ""}`);
  },
  createIssue: (owner: string, name: string, title: string, body: string) =>
    req<Issue>(`/users/${owner}/repos/${name}/issues`, {
      method: "POST",
      body: JSON.stringify({ title, body }),
    }),
  setIssueState: (owner: string, name: string, number: number, state: "open" | "closed") =>
    req<Issue>(`/users/${owner}/repos/${name}/issues/${number}`, {
      method: "PATCH",
      body: JSON.stringify({ state }),
    }),
  updateIssue: (
    owner: string,
    name: string,
    number: number,
    patch: { title?: string; body?: string; state?: "open" | "closed"; pinned?: boolean },
  ) =>
    req<Issue>(`/users/${owner}/repos/${name}/issues/${number}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }),
  deleteIssue: (owner: string, name: string, number: number) =>
    req<null>(`/users/${owner}/repos/${name}/issues/${number}`, { method: "DELETE" }),


  // issue/PR comments
  listComments: (
    owner: string,
    name: string,
    number: number,
    kind: "issues" | "pulls" = "issues",
  ) => req<IssueComment[]>(`/users/${owner}/repos/${name}/${kind}/${number}/comments`),
  postComment: (
    owner: string,
    name: string,
    number: number,
    body: string,
    kind: "issues" | "pulls" = "issues",
    loc?: { file_path: string; line: number; line_side: "old" | "new" },
  ) =>
    req<IssueComment>(`/users/${owner}/repos/${name}/${kind}/${number}/comments`, {
      method: "POST",
      body: JSON.stringify(loc ? { body, ...loc } : { body }),
    }),
  applySuggestion: (owner: string, name: string, number: number, commentId: number) =>
    req<{ sha: string; comment: IssueComment }>(
      `/users/${owner}/repos/${name}/pulls/${number}/comments/${commentId}/apply`,
      { method: "POST" },
    ),
  deleteComment: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/comments/${id}`, { method: "DELETE" }),


  // issue labels
  listLabels: (owner: string, name: string) => req<Label[]>(`/users/${owner}/repos/${name}/labels`),
  createLabel: (owner: string, name: string, labelName: string, color: string) =>
    req<Label>(`/users/${owner}/repos/${name}/labels`, {
      method: "POST",
      body: JSON.stringify({ name: labelName, color }),
    }),
  updateLabel: (owner: string, name: string, id: number, labelName: string, color: string) =>
    req<Label>(`/users/${owner}/repos/${name}/labels/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name: labelName, color }),
    }),
  deleteLabel: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/labels/${id}`, { method: "DELETE" }),
  setIssueLabels: (owner: string, name: string, number: number, labelIds: number[]) =>
    req<Issue>(`/users/${owner}/repos/${name}/issues/${number}/labels`, {
      method: "POST",
      body: JSON.stringify({ label_ids: labelIds }),
    }),


  // milestones
  listMilestones: (owner: string, name: string) =>
    req<Milestone[]>(`/users/${owner}/repos/${name}/milestones`),
  createMilestone: (owner: string, name: string, title: string, description: string) =>
    req<Milestone>(`/users/${owner}/repos/${name}/milestones`, {
      method: "POST",
      body: JSON.stringify({ title, description }),
    }),
  updateMilestone: (
    owner: string,
    name: string,
    id: number,
    fields: { title?: string; description?: string; state?: "open" | "closed" },
  ) =>
    req<Milestone>(`/users/${owner}/repos/${name}/milestones/${id}`, {
      method: "PATCH",
      body: JSON.stringify(fields),
    }),
  deleteMilestone: (owner: string, name: string, id: number) =>
    req<null>(`/users/${owner}/repos/${name}/milestones/${id}`, { method: "DELETE" }),
  setIssueMilestone: (owner: string, name: string, number: number, milestoneId: number) =>
    req<Issue>(`/users/${owner}/repos/${name}/issues/${number}/milestone`, {
      method: "POST",
      body: JSON.stringify({ milestone_id: milestoneId }),
    }),


  // 分支保护
  listBranchProtections: (owner: string, name: string) =>
    req<BranchProtection[]>(`/users/${owner}/repos/${name}/branch-protections`),
  setBranchProtection: (
    owner: string,
    name: string,
    branch: string,
    rule: {
      min_approvals: number;
      require_ci: boolean;
      require_codeowners: boolean;
      merge_queue: boolean;
      block_deletion: boolean;
      block_force_push: boolean;
    },
  ) =>
    req<BranchProtection>(
      `/users/${owner}/repos/${name}/branch-protections/${encodeURIComponent(branch)}`,
      { method: "PUT", body: JSON.stringify(rule) },
    ),
  deleteBranchProtection: (owner: string, name: string, branch: string) =>
    req<null>(`/users/${owner}/repos/${name}/branch-protections/${encodeURIComponent(branch)}`, {
      method: "DELETE",
    }),

  // PR CODEOWNERS 状态
  pullCodeowners: (owner: string, name: string, number: number) =>
    req<CodeownersStatus>(`/users/${owner}/repos/${name}/pulls/${number}/codeowners`),
  dequeuePull: (owner: string, name: string, number: number) =>
    req<{ dequeued: boolean }>(`/users/${owner}/repos/${name}/pulls/${number}/merge-queue`, {
      method: "DELETE",
    }),


  // pull requests
  listPulls: (owner: string, name: string, state?: PullState, limit?: number, offset?: number) => {
    const p = new URLSearchParams();
    if (limit != null) p.set("limit", String(limit));
    if (offset != null) p.set("offset", String(offset));
    if (state) p.set("state", state);
    const q = p.toString();
    return reqPage<PullRequest[]>(`/users/${owner}/repos/${name}/pulls${q ? `?${q}` : ""}`);
  },
  createPull: (
    owner: string,
    name: string,
    title: string,
    body: string,
    sourceBranch: string,
    targetBranch: string,
    draft = false,
  ) =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls`, {
      method: "POST",
      body: JSON.stringify({
        title,
        body,
        source_branch: sourceBranch,
        target_branch: targetBranch,
        draft,
      }),
    }),
  getPull: (owner: string, name: string, number: number) =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}`),
  pullDiff: (owner: string, name: string, number: number) =>
    req<PullDiff>(`/users/${owner}/repos/${name}/pulls/${number}/diff`),
  mergePull: (owner: string, name: string, number: number, method?: "merge" | "squash" | "rebase") =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}/merge`, {
      method: "POST",
      body: JSON.stringify(method ? { method } : {}),
    }),
  setPullState: (owner: string, name: string, number: number, state: "open" | "closed") =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}/state`, {
      method: "POST",
      body: JSON.stringify({ state }),
    }),
  setPullDraft: (owner: string, name: string, number: number, draft: boolean) =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}/draft`, {
      method: "POST",
      body: JSON.stringify({ draft }),
    }),
  setPullAutoMerge: (
    owner: string,
    name: string,
    number: number,
    enabled: boolean,
    method?: "fast-forward" | "merge" | "squash" | "rebase",
  ) =>
    req<PullRequest>(`/users/${owner}/repos/${name}/pulls/${number}/auto-merge`, {
      method: "POST",
      body: JSON.stringify({ enabled, method: method ?? "" }),
    }),


  // PR reviews
  listPullReviews: (owner: string, name: string, number: number) =>
    req<{ reviews: PullReview[]; summary: { approvals: number; request_changes: number }; gate?: MergeGate }>(
      `/users/${owner}/repos/${name}/pulls/${number}/reviews`,
    ),
  createPullReview: (owner: string, name: string, number: number, state: ReviewState, body?: string) =>
    req<PullReview>(`/users/${owner}/repos/${name}/pulls/${number}/reviews`, {
      method: "POST",
      body: JSON.stringify({ state, body: body?.trim() ? body.trim() : undefined }),
    }),

};
