package store

// ORM 模型：与表结构一一对应。公共 DTO（store.go 中的 User/Repo/...）保持原样，
// 各方法负责 row ↔ DTO 转换。

// ---- users & sessions ----

type userRow struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"uniqueIndex;size:255;not null"`
	Email        string `gorm:"size:255"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    string `gorm:"not null"`
	MFASecret    string `gorm:"not null;default:''"`
	MFAEnabled   bool   `gorm:"not null;default:false"`
	MFAMethod    string `gorm:"not null;default:'totp';size:16"` // totp | email
	NotifyEmail  bool   `gorm:"not null;default:false"`
	// 邮箱验证：仅对非空邮箱生效；token 24h 有效
	EmailVerified bool   `gorm:"not null;default:false"`
	EmailToken    string `gorm:"not null;default:'';size:255"`
	EmailTokenExp string `gorm:"not null;default:''"`
	// Banned 封禁标记：封禁用户禁止登录与一切 API/SSH 使用（系统专用 template 用户亦为封禁态）。
	Banned bool `gorm:"not null;default:false"`
}

func (userRow) TableName() string { return "users" }

type sessionRow struct {
	Token     string `gorm:"primaryKey;size:255"`
	UserID    int64  `gorm:"not null;index"`
	CreatedAt string `gorm:"not null"`
	ExpiresAt string `gorm:"not null;index"`
}

func (sessionRow) TableName() string { return "sessions" }

// ---- repos ----

type repoRow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	Owner       string `gorm:"not null;uniqueIndex:uq_repo;size:255"`
	Name        string `gorm:"not null;uniqueIndex:uq_repo;size:255"`
	Description string `gorm:"not null;default:''"`
	Private     bool   `gorm:"not null;default:true"`
	IsTemplate  bool   `gorm:"not null;default:false"`
	Banned      bool   `gorm:"not null;default:false"`
	// DefaultBranch 仓库默认分支（git HEAD 指向）；空值按 main 处理。
	DefaultBranch string `gorm:"not null;default:'main';size:255"`
	// HasIssues 是否启用 issue 功能（默认开启）；关闭后不可创建/修改 issue。
	HasIssues bool   `gorm:"not null;default:true"`
	CreatedAt string `gorm:"not null"`
}

func (repoRow) TableName() string { return "repos" }

// repoTopicRow 仓库话题/标签（一个仓库可挂多个 topic，用于 Explore 按标签搜索）。
type repoTopicRow struct {
	Owner string `gorm:"primaryKey;size:255"`
	Repo  string `gorm:"primaryKey;size:255"`
	Topic string `gorm:"primaryKey;size:64;index"`
}

func (repoTopicRow) TableName() string { return "repo_topics" }

// ---- ssh keys / gpg keys ----

type sshKeyRow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	UserID      int64  `gorm:"not null;index"`
	Name        string `gorm:"not null"`
	PublicKey   string `gorm:"not null"`
	Fingerprint string `gorm:"not null;uniqueIndex;size:255"`
	CreatedAt   string `gorm:"not null"`
}

func (sshKeyRow) TableName() string { return "ssh_keys" }

type gpgKeyRow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	UserID      int64  `gorm:"not null;index"`
	Fingerprint string `gorm:"not null;uniqueIndex;size:255"`
	Armor       string `gorm:"not null"`
	CreatedAt   string `gorm:"not null"`
}

func (gpgKeyRow) TableName() string { return "gpg_keys" }

type patRow struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	UserID     int64  `gorm:"not null;index"`
	OAuthAppID int64  `gorm:"column:oauth_app_id;index;default:0"` // 0 = 用户自建 PAT；>0 = OAuth 2.0 应用签发的 access token
	Name       string `gorm:"not null"`
	TokenHash  string `gorm:"not null;uniqueIndex;size:255"`
	Scopes     string `gorm:"not null;default:'repo'"`               // 逗号分隔: repo,inbox,keys
	CIDRs      string `gorm:"column:cidrs;not null;default:''"`      // 逗号分隔 IP/CIDR 白名单（去重、规范化）；空 = 不限来源
	ExpiresAt  string `gorm:"column:expires_at;not null;default:''"` // RFC3339 UTC 过期时间；空 = 永不过期
	CreatedAt  string `gorm:"not null"`
	LastUsedAt string `gorm:"not null;default:''"`
}

func (patRow) TableName() string { return "pats" }

// ---- pats DTO ----

type CreatedPAT struct {
	Token string `json:"token"` // 明文 token，仅创建响应中出现一次
	PAT          // 内联平铺
}

// ---- issues / labels / milestones ----

type issueRow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	Owner       string `gorm:"not null;uniqueIndex:uq_issue;size:255"`
	Repo        string `gorm:"not null;uniqueIndex:uq_issue;size:255"`
	Number      int64  `gorm:"not null;uniqueIndex:uq_issue"`
	Title       string `gorm:"not null"`
	Body        string `gorm:"not null;default:''"`
	State       string `gorm:"not null;default:'open';index:idx_issues_owner_repo"`
	Pinned      bool   `gorm:"not null;default:false"` // 置顶：列表最前
	Author      string `gorm:"not null"`
	CreatedAt   string `gorm:"not null"`
	UpdatedAt   string `gorm:"not null"`
	ClosedAt    *string
	MilestoneID *int64
}

func (issueRow) TableName() string { return "issues" }

// commentRow issue/PR 评论。kind 区分两种宿主（issue 与 PR 号码各自独立递增）。
type commentRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;index:idx_comment_host,priority:1;size:255"`
	Repo      string `gorm:"not null;index:idx_comment_host,priority:2;size:255"`
	Kind      string `gorm:"not null;index:idx_comment_host,priority:3;index:idx_comments_owner_repo;size:8"`
	Number    int64  `gorm:"not null;index:idx_comment_host,priority:4"`
	Author    string `gorm:"not null;size:255"`
	Body      string `gorm:"not null"`
	CreatedAt string `gorm:"not null"`
	UpdatedAt string `gorm:"not null"`

	// 行内评论（仅 PR，nil 表示普通评论）
	FilePath *string `gorm:"size:255"`
	Line     *int64
	LineSide string `gorm:"not null;default:'';size:8"`
}

func (commentRow) TableName() string { return "issue_comments" }

type repoLabelRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;uniqueIndex:uq_label;size:255"`
	Repo      string `gorm:"not null;uniqueIndex:uq_label;size:255"`
	Name      string `gorm:"not null;uniqueIndex:uq_label;size:255"`
	Color     string `gorm:"not null;default:'0366d6';size:32"`
	CreatedAt string `gorm:"not null"`
}

func (repoLabelRow) TableName() string { return "repo_labels" }

type issueLabelRow struct {
	IssueID int64 `gorm:"primaryKey;autoIncrement:false"`
	LabelID int64 `gorm:"primaryKey;autoIncrement:false"`
}

func (issueLabelRow) TableName() string { return "issue_labels" }

type milestoneRow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	Owner       string `gorm:"not null;index;size:255"`
	Repo        string `gorm:"not null;size:255"`
	Title       string `gorm:"not null"`
	Description string `gorm:"not null;default:''"`
	State       string `gorm:"not null;default:'open'"`
	CreatedAt   string `gorm:"not null"`
}

func (milestoneRow) TableName() string { return "milestones" }

// ---- projects（看板 + 泳道）----

type projectRow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	Owner       string `gorm:"not null;uniqueIndex:uq_project;size:255"`
	Repo        string `gorm:"not null;uniqueIndex:uq_project;size:255"`
	Name        string `gorm:"not null;uniqueIndex:uq_project;size:255"`
	Description string `gorm:"not null;default:''"`
	CreatedAt   string `gorm:"not null"`
}

func (projectRow) TableName() string { return "projects" }

type projectColumnRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	ProjectID int64  `gorm:"not null;index"`
	Name      string `gorm:"not null;size:255"`
	Position  int    `gorm:"not null;default:0"`
}

func (projectColumnRow) TableName() string { return "project_columns" }

type projectSwimlaneRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	ProjectID int64  `gorm:"not null;index"`
	Name      string `gorm:"not null;size:255"`
	Position  int    `gorm:"not null;default:0"`
}

func (projectSwimlaneRow) TableName() string { return "project_swimlanes" }

type projectCardRow struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	ProjectID  int64  `gorm:"not null;index"`
	ColumnID   int64  `gorm:"not null;index"`
	SwimlaneID int64  `gorm:"not null;default:0"`
	IssueNum   int64  `gorm:"not null;default:0"` // >0 = 关联 issue；0 = 纯文本卡片
	// Title 卡片名称；Note 为旧字段，保留用于向后兼容（与 Title 同步）。
	Title string `gorm:"not null;default:''"`
	Note  string `gorm:"not null;default:''"`
	// Body 卡片详情（Markdown 文本）。
	Body string `gorm:"not null;default:''"`
	// StartDate / DueDate 为甘特图用的日程（YYYY-MM-DD，空 = 未排期）。
	StartDate string `gorm:"not null;default:'';size:10"`
	DueDate   string `gorm:"not null;default:'';size:10"`
	Position  int    `gorm:"not null;default:0"`
	CreatedAt string `gorm:"not null"`
}

func (projectCardRow) TableName() string { return "project_cards" }

// ---- collabs / orgs ----

type collabRow struct {
	Owner      string `gorm:"primaryKey;size:255"`
	Repo       string `gorm:"primaryKey;size:255"`
	Username   string `gorm:"primaryKey;size:255;index:idx_repo_collabs_user"`
	Permission string `gorm:"not null;default:'read'"`
	CreatedAt  string `gorm:"not null"`
}

func (collabRow) TableName() string { return "repo_collabs" }

type orgRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"not null;uniqueIndex;size:255"`
	Display   string `gorm:"not null;default:''"`
	CreatedAt string `gorm:"not null"`
	Banned    bool   `gorm:"not null;default:false"`
}

func (orgRow) TableName() string { return "orgs" }

type orgMemberRow struct {
	Org       string `gorm:"primaryKey;size:255"`
	Username  string `gorm:"primaryKey;size:255;index:idx_org_members_user"`
	Role      string `gorm:"not null;default:'member'"`
	CreatedAt string `gorm:"not null"`
}

func (orgMemberRow) TableName() string { return "org_members" }

// orgFollowRow 用户关注组织（follower 关注 org）。单独建表，避免与用户关注混淆。
type orgFollowRow struct {
	Follower  string `gorm:"primaryKey;size:255;index:idx_org_follows_follower"`
	Org       string `gorm:"primaryKey;size:255;index:idx_org_follows_org"`
	CreatedAt string `gorm:"not null"`
}

func (orgFollowRow) TableName() string { return "org_follows" }

// ---- webhooks / admin / settings / oauth ----

type webhookRow struct {
	ID     int64  `gorm:"primaryKey;autoIncrement"`
	Owner  string `gorm:"not null;uniqueIndex:uq_hook;size:255"`
	Repo   string `gorm:"not null;uniqueIndex:uq_hook;size:255"`
	URL    string `gorm:"not null;uniqueIndex:uq_hook;size:1024"`
	Secret string `gorm:"not null;default:''"`
	// Events 逗号分隔的订阅事件类型；空 = 订阅全部事件。
	Events    string `gorm:"not null;default:''"`
	CreatedAt string `gorm:"not null"`
}

func (webhookRow) TableName() string { return "webhooks" }

// webhookDeliveryRow webhook 投递记录：每次投递（含重试）落一行，失败可重试。
// status: success | retry（待重试）| failed（超过次数上限，不再重试）
type webhookDeliveryRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	HookID    int64  `gorm:"not null;index"`
	Event     string `gorm:"not null;size:32"`
	Payload   string `gorm:"not null"` // 完整事件 JSON，重试时原样重发
	Status    string `gorm:"not null;size:16;index"`
	Code      int    `gorm:"not null;default:0"` // 最后一次 HTTP 状态码
	Error     string `gorm:"not null;default:''"`
	Attempts  int    `gorm:"not null;default:1"`
	NextRetry string `gorm:"not null;default:'';index"` // RFC3339，仅 retry 状态使用
	CreatedAt string `gorm:"not null"`
}

func (webhookDeliveryRow) TableName() string { return "webhook_deliveries" }

type adminUserRow struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"not null;uniqueIndex;size:255"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    string `gorm:"not null"`
}

func (adminUserRow) TableName() string { return "admin_users" }

type adminSessionRow struct {
	Token     string `gorm:"primaryKey;size:255"`
	AdminID   int64  `gorm:"not null;index"`
	CreatedAt string `gorm:"not null"`
	ExpiresAt string `gorm:"not null;index"`
}

func (adminSessionRow) TableName() string { return "admin_sessions" }

type settingRow struct {
	Key   string `gorm:"primaryKey;column:key;size:255"`
	Value string `gorm:"not null;default:''"`
}

func (settingRow) TableName() string { return "settings" }

type userOAuthRow struct {
	Provider   string `gorm:"primaryKey;size:64"`
	ExternalID string `gorm:"primaryKey;size:255"`
	UserID     int64  `gorm:"not null;index"`
	CreatedAt  string `gorm:"not null"`
}

func (userOAuthRow) TableName() string { return "user_oauth" }

// ---- stars / watches / notifications / forks / imports / mirrors ----

type starRow struct {
	Username  string `gorm:"primaryKey;size:255"`
	Owner     string `gorm:"primaryKey;size:255;index:idx_repo_stars_owner_repo"`
	Repo      string `gorm:"primaryKey;size:255;index:idx_repo_stars_owner_repo"`
	CreatedAt string `gorm:"not null"`
}

func (starRow) TableName() string { return "repo_stars" }

type watchRow struct {
	Username  string `gorm:"primaryKey;size:255"`
	Owner     string `gorm:"primaryKey;size:255;index:idx_repo_watches_owner_repo"`
	Repo      string `gorm:"primaryKey;size:255;index:idx_repo_watches_owner_repo"`
	CreatedAt string `gorm:"not null"`
}

func (watchRow) TableName() string { return "repo_watches" }

// followRow 用户关注关系（follower 关注 followee）。
type followRow struct {
	Follower  string `gorm:"primaryKey;size:255;index:idx_follows_follower"`
	Followee  string `gorm:"primaryKey;size:255;index:idx_follows_followee"`
	CreatedAt string `gorm:"not null"`
}

func (followRow) TableName() string { return "user_follows" }

type notificationRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Username  string `gorm:"not null;index:idx_notifications_user"`
	Kind      string `gorm:"not null"`
	Action    string `gorm:"not null"`
	Owner     string `gorm:"not null;size:255"`
	Repo      string `gorm:"not null;size:255"`
	Number    int64  `gorm:"not null"`
	Title     string `gorm:"not null;default:''"`
	Actor     string `gorm:"not null;default:''"`
	Read      bool   `gorm:"not null;default:false;index:idx_notifications_user"`
	CreatedAt string `gorm:"not null"`
}

func (notificationRow) TableName() string { return "notifications" }

type forkRow struct {
	Owner       string `gorm:"primaryKey;size:255"`
	Repo        string `gorm:"primaryKey;size:255"`
	SourceOwner string `gorm:"not null;size:255;index"`
	SourceRepo  string `gorm:"not null;size:255;index"`
	CreatedAt   string `gorm:"not null"`
}

func (forkRow) TableName() string { return "repo_forks" }

type importRow struct {
	Owner     string `gorm:"primaryKey;size:255"`
	Repo      string `gorm:"primaryKey;size:255"`
	SourceURL string `gorm:"not null"`
	Status    string `gorm:"not null;default:''"` // queued/running/synced/failed；空 = 旧数据已导入
	Error     string `gorm:"not null;default:''"` // 最近一次失败原因
	CreatedAt string `gorm:"not null"`
}

func (importRow) TableName() string { return "repo_imports" }

type mirrorRow struct {
	Owner      string `gorm:"primaryKey;size:255"`
	Repo       string `gorm:"primaryKey;size:255"`
	URL        string `gorm:"not null"`
	PrivateKey string `gorm:"not null;default:''"`
	Status     string `gorm:"not null;default:''"` // queued/running/synced/failed
	Error      string `gorm:"not null;default:''"` // 最近一次失败原因
	CreatedAt  string `gorm:"not null"`
}

func (mirrorRow) TableName() string { return "repo_mirrors" }

// ---- pull requests ----

type pullRequestRow struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	Owner        string `gorm:"not null;uniqueIndex:uq_pr;size:255"`
	Repo         string `gorm:"not null;uniqueIndex:uq_pr;size:255"`
	Number       int64  `gorm:"not null;uniqueIndex:uq_pr"`
	Title        string `gorm:"not null"`
	Body         string `gorm:"not null;default:''"`
	SourceBranch string `gorm:"not null"`
	TargetBranch string `gorm:"not null"`
	BaseSHA      string `gorm:"not null;default:''"`
	HeadSHA      string `gorm:"not null;default:''"`
	State        string `gorm:"not null;default:'open';index:idx_pulls_owner_repo"`
	Author       string `gorm:"not null"`
	CreatedAt    string `gorm:"not null"`
	UpdatedAt    string `gorm:"not null"`
	MergedAt     *string
	MergedBy     string `gorm:"not null;default:''"`
}

func (pullRequestRow) TableName() string { return "pull_requests" }

// pullReviewRow PR review（approve/request_changes/comment）。
// 同一 reviewer 重复提交插入新行，保留历史；当前状态取每个 reviewer 最新一条。
type pullReviewRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;index:idx_review_host,priority:1;size:255"`
	Repo      string `gorm:"not null;index:idx_review_host,priority:2;size:255"`
	Number    int64  `gorm:"not null;index:idx_review_host,priority:3"`
	Reviewer  string `gorm:"not null;index:idx_review_host,priority:4;size:255"`
	State     string `gorm:"not null;size:16"`
	Body      string `gorm:"not null;default:''"`
	CommitSHA string `gorm:"not null;default:'';size:64"`
	CreatedAt string `gorm:"not null"`
}

func (pullReviewRow) TableName() string { return "pull_reviews" }

// branchProtectionRow 分支保护规则（per 分支；SSH push / 删除 / 合并门禁共用）。
type branchProtectionRow struct {
	Owner          string `gorm:"primaryKey;size:255;column:owner"`
	Repo           string `gorm:"primaryKey;size:255;column:repo"`
	Branch         string `gorm:"primaryKey;size:255;column:branch"`
	MinApprovals   int    `gorm:"not null;default:0;column:min_approvals"`  // 合并门禁：需要的最少 approve 数
	RequireCI      bool   `gorm:"not null;default:false;column:require_ci"` // 合并门禁：要求 head 的 CI 通过
	BlockDeletion  bool   `gorm:"not null;default:true;column:block_deletion"`
	BlockForcePush bool   `gorm:"not null;default:true;column:block_force_push"`
	CreatedAt      string `gorm:"not null"`
}

func (branchProtectionRow) TableName() string { return "branch_protections" }

// ---- pipelines ----

type pipelineCfgRow struct {
	Owner     string `gorm:"primaryKey;size:255"`
	Repo      string `gorm:"primaryKey;size:255"`
	Enabled   bool   `gorm:"not null;default:false"`
	CreatedAt string `gorm:"not null"`
}

func (pipelineCfgRow) TableName() string { return "repo_pipelines" }

type pipelineRunRow struct {
	ID    int64  `gorm:"primaryKey;autoIncrement"`
	Owner string `gorm:"not null;index:idx_pipeline_runs_repo;size:255"`
	Repo  string `gorm:"not null;index:idx_pipeline_runs_repo;size:255"`
	// File 流水线定义文件路径（如 .gitdash.yml 或 .gitdash/ci.yml）；旧记录为空。
	File      string `gorm:"column:file;not null;default:'';size:255"`
	SHA       string `gorm:"column:sha;not null;default:''"`
	Ref       string `gorm:"not null;default:''"`
	TriggerBy string `gorm:"column:trigger_by;not null;default:''"`
	// Event 触发事件：push|pull_request|schedule|workflow_dispatch|manual
	Event string `gorm:"not null;default:''"`
	// RunAt 延迟执行时间（RFC3339）；空 = 立即执行。
	RunAt string `gorm:"column:run_at;not null;default:''"`
	// Inputs dispatch 传入的键值对（JSON；重跑时复用），其余事件为空
	Inputs     string `gorm:"not null;default:''"`
	Status     string `gorm:"not null;default:'pending'"`
	StepsTotal int    `gorm:"not null;default:0"`
	StepsDone  int    `gorm:"not null;default:0"`
	Error      string `gorm:"not null;default:''"`
	CreatedAt  string `gorm:"not null"`
	FinishedAt *string
	// RunnerName 由远程 runner 执行时记录（builtin 执行为空）
	RunnerName string `gorm:"column:runner_name;not null;default:''"`
}

func (pipelineRunRow) TableName() string { return "pipeline_runs" }

// pipelineScheduleRow 定时触发去重：每个 (repo, file, cron) 记录最近一次触发时间，
// 多实例部署时经条件更新做原子 claim，避免重复触发。
type pipelineScheduleRow struct {
	Owner     string `gorm:"primaryKey;size:255"`
	Repo      string `gorm:"primaryKey;size:255"`
	File      string `gorm:"primaryKey;column:file;size:255"`
	Expr      string `gorm:"primaryKey;size:128"`
	LastFired string `gorm:"not null;default:''"`
}

func (pipelineScheduleRow) TableName() string { return "pipeline_schedules" }

// repoEnvVarRow 仓库级流水线环境变量（每次运行注入容器 / host 执行环境）。
type repoEnvVarRow struct {
	Owner     string `gorm:"primaryKey;size:255"`
	Repo      string `gorm:"primaryKey;size:255"`
	Key       string `gorm:"primaryKey;size:255"`
	Value     string `gorm:"not null;default:''"`
	CreatedAt string `gorm:"not null"`
}

func (repoEnvVarRow) TableName() string { return "repo_env_vars" }

// repoSecretRow 仓库 CI secret（值经 AES-GCM 加密或明文存储，见 secrets.go）。
type repoSecretRow struct {
	Owner     string `gorm:"primaryKey;size:255"`
	Repo      string `gorm:"primaryKey;size:255"`
	Name      string `gorm:"primaryKey;size:255"`
	Value     string `gorm:"not null;default:''"`
	CreatedAt string `gorm:"not null"`
	UpdatedAt string `gorm:"not null;default:''"`
}

func (repoSecretRow) TableName() string { return "repo_secrets" }

// ---- releases / release assets ----

type releaseRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;uniqueIndex:uq_release;size:255"`
	Repo      string `gorm:"not null;uniqueIndex:uq_release;size:255"`
	TagName   string `gorm:"column:tag_name;not null;uniqueIndex:uq_release;size:255"`
	Name      string `gorm:"not null;default:''"`
	Body      string `gorm:"not null;default:''"`
	Author    string `gorm:"not null;size:255"`
	CreatedAt string `gorm:"not null"`
}

func (releaseRow) TableName() string { return "releases" }

type releaseAssetRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;index:idx_release_assets;size:255"`
	Repo      string `gorm:"not null;index:idx_release_assets;size:255"`
	ReleaseID int64  `gorm:"column:release_id;not null;uniqueIndex:uq_release_asset;index:idx_release_assets"`
	Filename  string `gorm:"not null;uniqueIndex:uq_release_asset;size:255"`
	Size      int64  `gorm:"not null;default:0"`
	Content   []byte `gorm:"not null"`
	CreatedAt string `gorm:"not null"`
}

func (releaseAssetRow) TableName() string { return "release_assets" }

// ---- byok（bring your own key：用户自带的 LLM 密钥）----

// byokKeyRow 用户配置的一个 BYOK 密钥（provider + api_key + 可选 base_url/model）。
// api_key 只存库，任何读接口都不回传。
type byokKeyRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Username  string `gorm:"not null;index;size:255"`
	Name      string `gorm:"not null;size:64"`
	Provider  string `gorm:"not null;size:32"`
	APIKey    string `gorm:"not null;size:1024"`
	BaseURL   string `gorm:"not null;default:'';size:1024"`
	Model     string `gorm:"not null;default:'';size:255"`
	CreatedAt string `gorm:"not null"`
	UpdatedAt string `gorm:"not null"`
}

func (byokKeyRow) TableName() string { return "byok_keys" }

// ---- user avatars ----

// userAvatarRow 用户头像图片（单独表，避免 users 常规查询加载大字段）。
type userAvatarRow struct {
	Username    string `gorm:"primaryKey;size:255"`
	ContentType string `gorm:"not null;default:'';size:64"`
	Data        []byte `gorm:"not null"`
	UpdatedAt   string `gorm:"not null"`
}

func (userAvatarRow) TableName() string { return "user_avatars" }

// ---- copilot sessions（嵌入式 agent，工作区为仓库的本地克隆副本）----

// copilotSessionRow 仓库内的一个 AI copilot 会话，绑定到某个 BYOK 密钥。
// 会话由进程内嵌的 holihur/agent 驱动，无需 Docker；gitdash 在每轮
// 对话结束后自动把工作区改动提交并推送到 CopilotBranch。
type copilotSessionRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;index;size:255"`
	Repo      string `gorm:"not null;index;size:255"`
	CreatedBy string `gorm:"not null;size:255"`
	ByokID    int64  `gorm:"not null;default:0"`
	// IssueNumber 为会话关联的 issue 编号（0 = 未关联）；关联后会话分支会在闭环
	// 结束时自动开 PR，并在 PR 正文里 Closes 该 issue。
	IssueNumber int64 `gorm:"not null;default:0"`
	// PRNumber 为自动开出的 PR 编号（0 = 尚未开 PR），用于去重。
	PRNumber int64  `gorm:"not null;default:0"`
	Prompt   string `gorm:"not null;default:''"` // 额外 system 指令（可选）
	// Branch 为会话工作区分支（copilot/session-<id>），HeadSHA 为最近推送的提交。
	Branch  string `gorm:"not null;default:'';size:255"`
	HeadSHA string `gorm:"not null;default:'';size:64"`
	Status  string `gorm:"not null;default:'idle'"` // idle | running | failed
	Error   string `gorm:"not null;default:''"`
	// 旧字段保留以兼容既有 SQLite/PG 库，嵌入式 agent 不再使用。
	Image     string `gorm:"not null;default:'';size:255"`
	Command   string `gorm:"not null;default:''"`
	CreatedAt string `gorm:"not null"`
	UpdatedAt string `gorm:"not null"`
}

func (copilotSessionRow) TableName() string { return "copilot_sessions" }

// ---- OAuth 2.0 provider（gitdash 作为授权服务器）----

// oauthAppRow 用户注册的第三方 OAuth 应用（client_id 公开，client_secret 只存哈希）。
type oauthAppRow struct {
	ID               int64  `gorm:"primaryKey;autoIncrement"`
	UserID           int64  `gorm:"not null;index"`
	Name             string `gorm:"not null"`
	Homepage         string `gorm:"not null;default:''"`
	Description      string `gorm:"not null;default:''"`
	CallbackURL      string `gorm:"column:callback_url;not null;size:1024"`
	ClientID         string `gorm:"column:client_id;not null;uniqueIndex;size:64"`
	ClientSecretHash string `gorm:"column:client_secret_hash;not null;size:255"`
	CreatedAt        string `gorm:"not null"`
}

func (oauthAppRow) TableName() string { return "oauth_apps" }

// oauthGrantRow 授权码（authorization code）：一次性、10 分钟有效。
type oauthGrantRow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	CodeHash    string `gorm:"column:code_hash;not null;uniqueIndex;size:255"`
	AppID       int64  `gorm:"column:app_id;not null;index"`
	UserID      int64  `gorm:"column:user_id;not null;index"`
	Scopes      string `gorm:"not null;default:'repo'"` // 逗号分隔: repo,inbox,keys
	RedirectURI string `gorm:"column:redirect_uri;not null;size:1024"`
	ExpiresAt   string `gorm:"column:expires_at;not null;index"`
	CreatedAt   string `gorm:"not null"`
}

func (oauthGrantRow) TableName() string { return "oauth_grants" }

// oauthDeviceGrantRow OAuth 2.0 设备流（RFC 8628）的设备授权记录。
// status: pending（等待用户确认）| approved（已授权，待 token 换发）| denied（已拒绝）。
type oauthDeviceGrantRow struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	DeviceCodeHash string `gorm:"column:device_code_hash;not null;uniqueIndex;size:255"`
	UserCode       string `gorm:"column:user_code;not null;uniqueIndex;size:32"`
	ClientID       string `gorm:"column:client_id;not null;index"`
	Scopes         string `gorm:"not null;default:'repo'"`
	Status         string `gorm:"not null;default:'pending';size:16"`
	UserID         int64  `gorm:"column:user_id;not null;default:0;index"`
	ExpiresAt      string `gorm:"column:expires_at;not null;index"`
	CreatedAt      string `gorm:"not null"`
}

func (oauthDeviceGrantRow) TableName() string { return "oauth_device_grants" }

// incomingWebhookRow 仓库的入站 webhook：外部系统在请求中携带 token（X-Gitdash-Token）
// 调用即可创建 issue。TokenHash 为 token 的 sha256；明文仅在创建/轮换时返回一次。
type incomingWebhookRow struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	Owner      string `gorm:"not null;uniqueIndex:uq_incoming;size:255"`
	Repo       string `gorm:"not null;uniqueIndex:uq_incoming;size:255"`
	TokenHash  string `gorm:"not null;uniqueIndex;size:255"`
	CreatedAt  string `gorm:"not null"`
	LastUsedAt string `gorm:"not null;default:''"`
}

func (incomingWebhookRow) TableName() string { return "incoming_webhooks" }

// ipBanRow 管理端 IP / CIDR 黑名单：命中来源 IP 的请求（HTTP 与 SSH）一律拒绝。
type ipBanRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	CIDR      string `gorm:"column:cidr;not null;uniqueIndex;size:64"`
	Note      string `gorm:"not null;default:''"`
	CreatedBy string `gorm:"not null;default:''"`
	CreatedAt string `gorm:"not null"`
}

func (ipBanRow) TableName() string { return "ip_bans" }
