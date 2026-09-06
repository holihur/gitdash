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
	NotifyEmail  bool   `gorm:"not null;default:false"`
	// 邮箱验证：仅对非空邮箱生效；token 24h 有效
	EmailVerified bool   `gorm:"not null;default:false"`
	EmailToken    string `gorm:"not null;default:'';size:255"`
	EmailTokenExp string `gorm:"not null;default:''"`
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
	CreatedAt   string `gorm:"not null"`
}

func (repoRow) TableName() string { return "repos" }

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
	Name       string `gorm:"not null"`
	TokenHash  string `gorm:"not null;uniqueIndex;size:255"`
	Scopes     string `gorm:"not null;default:'repo'"` // 逗号分隔: repo,inbox,keys
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
}

func (orgRow) TableName() string { return "orgs" }

type orgMemberRow struct {
	Org       string `gorm:"primaryKey;size:255"`
	Username  string `gorm:"primaryKey;size:255;index:idx_org_members_user"`
	Role      string `gorm:"not null;default:'member'"`
	CreatedAt string `gorm:"not null"`
}

func (orgMemberRow) TableName() string { return "org_members" }

// ---- webhooks / admin / settings / oauth ----

type webhookRow struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Owner     string `gorm:"not null;uniqueIndex:uq_hook;size:255"`
	Repo      string `gorm:"not null;uniqueIndex:uq_hook;size:255"`
	URL       string `gorm:"not null;uniqueIndex:uq_hook;size:1024"`
	Secret    string `gorm:"not null;default:''"`
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
	MinApprovals   int    `gorm:"not null;default:0;column:min_approvals"` // 合并门禁：需要的最少 approve 数
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
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	Owner      string `gorm:"not null;index:idx_pipeline_runs_repo;size:255"`
	Repo       string `gorm:"not null;index:idx_pipeline_runs_repo;size:255"`
	SHA        string `gorm:"column:sha;not null;default:''"`
	Ref        string `gorm:"not null;default:''"`
	TriggerBy  string `gorm:"column:trigger_by;not null;default:''"`
	Status     string `gorm:"not null;default:'pending'"`
	StepsTotal int    `gorm:"not null;default:0"`
	StepsDone  int    `gorm:"not null;default:0"`
	Error      string `gorm:"not null;default:''"`
	CreatedAt  string `gorm:"not null"`
	FinishedAt *string
}

func (pipelineRunRow) TableName() string { return "pipeline_runs" }

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
