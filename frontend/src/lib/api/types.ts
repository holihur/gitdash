// API 响应类型（按域拆分的模块共用的类型集中于此）。

export interface User {
  username: string;
  email?: string;
  created_at: string;
  mfa_enabled: boolean;
  email_verified?: boolean;
  avatar_url?: string;
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

export interface DockerImage {
  name: string;
  tags: string[];
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
  /** 仓库默认分支（git HEAD） */
  default_branch?: string;
  /** 是否启用 issue 功能 */
  has_issues?: boolean;
  /** 仅“可访问仓库列表”返回：owner / read / write */
  role?: "owner" | "read" | "write";
  /** star 数量与当前用户是否已 star */
  stars?: number;
  starred?: boolean;
  /** watch 数量与当前用户是否已 watch */
  watchers?: number;
  watching?: boolean;
  /** 仓库磁盘占用（字节，仅仓库详情接口返回） */
  size?: number;
  /** fork 来源（仅 fork 仓库） */
  fork_owner?: string;
  fork_repo?: string;
  /** 导入任务状态（queued/running/synced/failed），非导入仓库为空 */
  import_status?: "queued" | "running" | "synced" | "failed";
  /** 最近一次导入失败原因 */
  import_error?: string;
  /** 仓库标签/话题 */
  topics?: string[];
}

/** `git gc` 执行结果（字节） */
export interface RepoGCResult {
  before_bytes: number;
  after_bytes: number;
  freed_bytes: number;
}

export interface TopicCount {
  topic: string;
  count: number;
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

export interface OrgProfile {
  name: string;
  display: string;
  created_at: string;
  /** 当前用户在该组织中的角色：""（非成员）/ member / owner */
  role: string;
  members: OrgMember[];
  /** 可见仓库：成员含私有，非成员仅公开 */
  repos: Repo[];
  followers: number;
  is_following: boolean;
}

export interface OrgFollowState {
  followers: number;
  is_following: boolean;
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
  cidrs: string[];
  expires_at: string;
  created_at: string;
  last_used_at: string;
}

export interface CreatedPAT extends PAT {
  token: string;
}

export interface OAuthApp {
  id: number;
  name: string;
  homepage: string;
  description: string;
  callback_url: string;
  client_id: string;
  created_at: string;
}

export interface CreatedOAuthApp extends OAuthApp {
  client_secret: string;
}

export interface OAuthAuthorization {
  id: number;
  app_id: number;
  app_name: string;
  scopes: string[];
  created_at: string;
  last_used_at: string;
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
  /** 该文件最近一次变更的提交（服务端填充） */
  latest_commit?: Commit;
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
  pinned?: boolean;
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
  /** 卡片名称 */
  title: string | null;
  /** 卡片详情（Markdown） */
  body: string | null;
  /** 兼容字段：与 title 同步 */
  note: string | null;
  /** 甘特图日程（YYYY-MM-DD，空串 = 未排期） */
  start_date: string;
  due_date: string;
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
  /** 非空表示该评论内的 suggestion 已被应用（提交 SHA） */
  suggestion_applied_sha?: string;
}

export type ReviewState = "approve" | "request_changes" | "comment";

export interface BranchProtection {
  owner: string;
  repo: string;
  branch: string;
  min_approvals: number;
  require_ci: boolean;
  require_codeowners: boolean;
  merge_queue: boolean;
  block_deletion: boolean;
  block_force_push: boolean;
}

export interface MergeGate {
  required: number;
  approvals: number;
  mergeable: boolean;
  /** 分支保护要求 CI 通过时附带 */
  ci_required?: boolean;
  ci_status?: string;
  /** 分支保护要求 CODEOWNERS 批准时附带 */
  codeowners_required?: boolean;
  codeowners?: string[];
  codeowners_approved?: string[];
  codeowners_missing?: string[];
}

/** PR 的 CODEOWNERS 状态 */
export interface CodeownersStatus {
  files: { path: string; owners: string[] }[];
  owners: string[];
  approved: string[];
  missing: string[];
  satisfied: boolean;
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

/** 全局代码搜索结果条目（跨仓库）。 */
export interface CodeSearchHit {
  owner: string;
  repo: string;
  path: string;
  line: number;
  text: string;
}

export interface CodeSearchResponse {
  results: CodeSearchHit[];
  truncated: boolean;
  repos_searched: number;
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
  /** 草稿 PR（未准备好合并） */
  draft: boolean;
  /** 开启后合并门禁满足时自动合并 */
  auto_merge: boolean;
  auto_merge_method?: string;
  /** 已加入目标分支的合并队列 */
  merge_queued?: boolean;
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
  /** 订阅的事件类型；空数组 = 全部事件 */
  events: string[];
  created_at: string;
}

/** 入站 webhook：外部系统凭 token 创建 issue。 */
export interface IncomingWebhook {
  enabled: boolean;
  /** 调用路径（如 /api/hooks/incoming/owner/repo），仅配置后返回 */
  path?: string;
  created_at?: string;
  last_used_at?: string;
  /** token 明文仅在创建/轮换响应中返回一次 */
  token?: string;
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

export type PipelineRunStatus = "pending" | "running" | "success" | "failed" | "cancelled";

export interface PipelineRun {
  id: number;
  /** 流水线定义文件（多文件支持；旧记录可能为空） */
  file?: string;
  sha: string;
  ref: string;
  trigger_by: string;
  status: PipelineRunStatus;
  steps_total: number;
  steps_done: number;
  /** 触发事件：push|pull_request|schedule|workflow_dispatch|manual（旧记录可能为空） */
  event?: string;
  /** 延迟执行时间（RFC3339）；空 = 立即执行 */
  run_at?: string;
  /** 仅 workflow_dispatch 传入的 inputs */
  inputs?: Record<string, string>;
  error?: string;
  created_at: string;
  finished_at: string | null;
  runner_name?: string;
  /** 该运行是否存有可下载的归档产物 */
  has_artifacts?: boolean;
  /** 仅详情返回 */
  log?: string;
}

/** 运行产物中的一个文件条目 */
export interface ArtifactFile {
  path: string;
  size: number;
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

/** 仓库 CI secret 元信息（永不包含明文值）。 */
export interface RepoSecret {
  name: string;
  updated_at: string;
}

export interface PipelineGraphNode {
  id: string;
  label: string;
  kind: "start" | "step" | "parallel" | "sub" | "end";
  when?: string;
}

export interface PipelineGraphEdge {
  from: string;
  to: string;
}

export interface PipelineGraph {
  ref: string;
  /** 该图对应的流水线文件 */
  file?: string;
  image?: string;
  timeout?: string;
  graph: { image?: string; nodes: PipelineGraphNode[]; edges: PipelineGraphEdge[] };
}

export interface ByokKey {
  id: number;
  name: string;
  provider: string;
  base_url?: string;
  model?: string;
  key_set: boolean;
  created_at: string;
  updated_at: string;
}

export type CopilotStatus = "idle" | "running" | "failed" | "created" | "stopped";

export interface CopilotSession {
  id: number;
  created_by: string;
  byok_id: number;
  issue_number?: number;
  pr_number?: number;
  prompt: string;
  branch?: string;
  head_sha?: string;
  status: CopilotStatus;
  error?: string;
  created_at: string;
  updated_at: string;
}

/** 一次工具调用（对话历史里展示）。 */
export interface CopilotTool {
  name: string;
  input?: string;
  result?: string;
  is_error?: boolean;
}

/** copilot 对话历史里的一条消息。 */
export interface CopilotMessage {
  role: "user" | "assistant";
  text?: string;
  tools?: CopilotTool[];
}

export type NotifKind = "issue" | "pull" | "system";
export type NotifAction =
  | "opened"
  | "closed"
  | "reopened"
  | "merged"
  | "ready_for_review"
  | "converted_to_draft"
  | "banned_user"
  | "banned_repo"
  | "banned_org";

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

// ---- 用户主页与关注 ----

export interface UserSummary {
  username: string;
  created_at: string;
}

export interface UserProfile {
  username: string;
  created_at: string;
  followers: number;
  following: number;
  /** 当前登录用户是否就是该用户本人 */
  is_self: boolean;
  is_following: boolean;
  /** 可见仓库：本人含私有，他人仅公开 */
  repos: Repo[];
  avatar_url?: string;
}

export interface FollowState {
  followers: number;
  following: number;
  is_following: boolean;
}
