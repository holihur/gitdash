package store

// ORM 模型：与表结构一一对应。公共 DTO（store.go 中的 User/Repo/...）保持原样，
// 各方法负责 row ↔ DTO 转换。

// ---- users & sessions ----

type userRow struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"uniqueIndex;size:255;not null"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    string `gorm:"not null"`
	MFASecret    string `gorm:"not null;default:''"`
	MFAEnabled   bool   `gorm:"not null;default:false"`
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
	CreatedAt string `gorm:"not null"`
}

func (importRow) TableName() string { return "repo_imports" }

type mirrorRow struct {
	Owner      string `gorm:"primaryKey;size:255"`
	Repo       string `gorm:"primaryKey;size:255"`
	URL        string `gorm:"not null"`
	PrivateKey string `gorm:"not null;default:''"`
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
