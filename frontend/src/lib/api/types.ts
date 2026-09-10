// API 响应类型（按域拆分的模块共用的类型集中于此）。

export interface User {
  username: string;
  email?: string;
  created_at: string;
  mfa_enabled: boolean;
  email_verified?: boolean;
}

export interface PackageEntry {
  id: number;
  owner: string;
  repo?: string;
  type: string;
  name: string;
  version: string;
  filename: string;
  size: number;
  checksum: string;
  downloads: number;
  uploader: string;
  created_at: string;
}

export interface PackageAuditEntry {
  id: number;
  owner: string;
  type: string;
  name: string;
  version: string;
  action: string;
  actor: string;
  created_at: string;
}

export interface LoginResult {
  token?: string;
  username?: string;
  mfa_required?: boolean;
  mfa_token?: string;
  mfa_method?: "totp" | "email";
}

export interface MFAStatus {
  enabled: boolean;
  method?: "totp" | "email";
  pending_secret?: string;
  otpauth_url?: string;
}

export interface MFAEnroll {
  secret: string;
  otpauth_url: string;
}

export interface Repo {
  id: number;
  owner: string;
  name: string;
  description: string;
  created_at: string;
  private?: boolean;
  is_template?: boolean;
  /** 仅“可访问仓库列表”返回：owner / read / write */
  role?: "owner" | "read" | "write";
  /** star 数量与当前用户是否已 star */
  stars?: number;
  starred?: boolean;
  /** watch 数量与当前用户是否已 watch */
  watchers?: number;
  watching?: boolean;
  /** fork 来源（仅 fork 仓库） */
  fork_owner?: string;
  fork_repo?: string;
  /** 导入任务状态（queued/running/synced/failed），非导入仓库为空 */
  import_status?: "queued" | "running" | "synced" | "failed";
  /** 最近一次导入失败原因 */
  import_error?: string;
}

export interface GPGKey {
  id: number;
  fingerprint: string;
  created_at: string;
}

export interface Org {
  name: string;
  display: string;
  created_at: string;
  role: string;
}

export interface OrgMember {
  org: string;
  username: string;
  role: string;
}

export interface SSHKey {
  id: number;
  name: string;
  public_key: string;
  fingerprint: string;
  created_at: string;
}

export interface PAT {
  id: number;
  name: string;
  scopes: string[];
  created_at: string;
  last_used_at: string;
}

export interface CreatedPAT extends PAT {
  token: string;
}

export interface Branch {
  name: string;
  is_head: boolean;
}

export interface TreeEntry {
  name: string;
  type: "blob" | "tree";
  mode: string;
  size: number;
  sha: string;
  modified_at?: string;
  modified_by?: string;
  modified_msg?: string;
  last_commit?: string;
}

export interface Blob {
  path: string;
  size: number;
  encoding: "utf-8" | "binary" | "truncated";
  content: string;
}

export interface BlameCommit {
  sha: string;
  author: string;
  date: string;
  message: string;
}

export interface BlameLine {
  line: number;
  commit: string;
  content: string;
}

export interface Blame {
  path: string;
  commits: Record<string, BlameCommit>;
  lines: BlameLine[];
}

export interface Commit {
  sha: string;
  author: string;
  date: string;
  message: string;
  /** 经已注册 GPG 公钥验证的提交所属用户 */
  gpg_verified?: string;
  /** GPG 签名状态：verified | unknown_key | invalid（无签名字段缺省） */
  gpg_status?: "verified" | "unknown_key" | "invalid";
}
export interface Issue {
  id: number;
  number: number;
  title: string;
  body: string;
  state: "open" | "closed";
  author: string;
  created_at: string;
  updated_at: string;
  closed_at: string | null;
  /** 服务端 enrich：所属标签与里程碑 */
  labels?: Label[];
  milestone?: Milestone | null;
}

export interface Label {
  id: number;
  name: string;
  color: string;
}

export interface Milestone {
  id: number;
  title: string;
  description: string;
  state: "open" | "closed";
  open_issues: number;
  closed_issues: number;
}

export interface Project {
  id: number;
  owner: string;
  repo: string;
  name: string;
  description: string;
  card_count: number;
  created_at: string;
}

export interface ProjectColumn {
  id: number;
  project_id: number;
  name: string;
  position: number;
}

export interface ProjectSwimlane {
  id: number;
  project_id: number;
  name: string;
  position: number;
}

export interface ProjectCard {
  id: number;
  project_id: number;
  column_id: number;
  swimlane_id: number;
  issue_number: number | null;
  issue_title: string | null;
  issue_state: string | null;
  note: string | null;
  position: number;
  created_at: string;
}

export interface ProjectBoard {
  project: Project;
  columns: ProjectColumn[];
  swimlanes: ProjectSwimlane[];
  cards: ProjectCard[];
}

export interface IssueComment {
  id: number;
  number: number;
  author: string;
  body: string;
  created_at: string;
  updated_at: string;
  /** PR 行内评论才有值 */
  file_path?: string | null;
  line?: number | null;
  line_side?: "old" | "new" | null;
}

export type ReviewState = "approve" | "request_changes" | "comment";

export interface BranchProtection {
  owner: string;
  repo: string;
  branch: string;
  min_approvals: number;
  block_deletion: boolean;
  block_force_push: boolean;
}

export interface MergeGate {
  required: number;
  approvals: number;
  mergeable: boolean;
}

export interface PullReview {
  id: number;
  reviewer: string;
  state: ReviewState;
  body: string;
  commit_sha: string;
  created_at: string;
}

export interface SearchResult {
  path: string;
  line: number;
  text: string;
}

export interface ReleaseAsset {
  filename: string;
  size: number;
  created_at: string;
}

export interface Release {
  tag_name: string;
  name: string;
  body: string;
  created_at: string;
  assets: ReleaseAsset[];
}

export interface Collab {
  owner: string;
  repo: string;
  username: string;
  permission: "read" | "write";
  created_at: string;
}

export type PullState = "open" | "merged" | "closed";

export interface PullRequest {
  id: number;
  number: number;
  title: string;
  body: string;
  source_branch: string;
  target_branch: string;
  base_sha: string;
  head_sha: string;
  state: PullState;
  author: string;
  created_at: string;
  updated_at: string;
  merged_at: string | null;
  merged_by: string;
  /** API enrich（open PR）：可合并性预检与 head 提交 CI 状态 */
  mergeable?: boolean;
  conflicted?: boolean;
  ci?: { run_id: number; status: "pending" | "running" | "success" | "failed" };
}

export interface Tag {
  name: string;
  sha: string;
  message: string;
}

export interface PullDiff {
  files: { path: string; status: "A" | "M" | "D"; insertions: number; deletions: number }[];
  patch: string;
  base_sha: string;
  head_sha: string;
}

export interface Webhook {
  id: number;
  owner: string;
  repo: string;
  url: string;
  created_at: string;
}

export interface GlobalSearchResult {
  repos: Repo[];
  issues: (Omit<Issue, "labels" | "milestone"> & { owner: string; repo: string })[];
  users: { kind: "user" | "org"; name: string; display?: string; created_at: string }[];
}

export interface WebhookDelivery {
  id: number;
  hook_id: number;
  event: string;
  status: "success" | "retry" | "failed";
  code: number;
  error?: string;
  attempts: number;
  next_retry?: string;
  created_at: string;
}

export type PipelineRunStatus = "pending" | "running" | "success" | "failed";

export interface PipelineRun {
  id: number;
  sha: string;
  ref: string;
  trigger_by: string;
  status: PipelineRunStatus;
  steps_total: number;
  steps_done: number;
  error?: string;
  created_at: string;
  finished_at: string | null;
  runner_name?: string;
  /** 仅详情返回 */
  log?: string;
}

export interface Runner {
  id: number;
  name: string;
  labels: string[];
  scope: string;
  status: string;
  /** ""=agent 主动外连 | "reverse"=runner 监听、服务端拨号 */
  mode?: string;
  /** 反向模式：服务端拨号的 WS 地址 */
  url?: string;
  last_seen?: string | null;
  created_at: string;
}

export interface RepoEnvVar {
  key: string;
  value: string;
  created_at: string;
}

export type NotifKind = "issue" | "pull";
export type NotifAction = "opened" | "closed" | "reopened" | "merged";

export interface Notification {
  id: number;
  kind: NotifKind;
  action: NotifAction;
  owner: string;
  repo: string;
  number: number;
  title: string;
  actor: string;
  read: boolean;
  created_at: string;
}
