package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	gormsqlite "github.com/glebarez/sqlite" // 纯 Go sqlite GORM dialector
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

// ---- public DTO（保持原有 JSON 形状，api 层无感知） ----

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

type UserAuth struct {
	ID           int64
	Username     string
	Email        string
	PasswordHash string
	CreatedAt    string
	MFASecret    string // 空 = 未启用/无待激活 secret
	MFAEnabled   bool
	NotifyEmail  bool // 邮件通知开关（默认关）
}

type Repo struct {
	ID          int64  `json:"id"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	CreatedAt   string `json:"created_at"`
	// Role 仅用于“可访问仓库列表”（owner / read / write），普通查询为空
	Role string `json:"role,omitempty"`
	// 展示字段（由 API 层填充，store 查询不扫描）
	Stars     int    `json:"stars"`
	Starred   bool   `json:"starred"`
	Watchers  int    `json:"watchers"`
	Watching  bool   `json:"watching"`
	ForkOwner string `json:"fork_owner,omitempty"`
	ForkRepo  string `json:"fork_repo,omitempty"`
	ImportURL string `json:"import_url,omitempty"`
	// 导入任务状态（queued/running/synced/failed），非导入仓库为空
	ImportStatus string `json:"import_status,omitempty"`
}

// Mirror 仓库 push 镜像目标（同步到 GitHub/GitLab 等第三方）。
type Mirror struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	URL        string `json:"url"`
	PrivateKey string `json:"-"` // 认证私钥，不回传
	Status     string `json:"status,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type SSHKey struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}

type Issue struct {
	ID        int64   `json:"id"`
	Owner     string  `json:"-"`
	Repo      string  `json:"-"`
	Number    int64   `json:"number"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	State     string  `json:"state"` // "open" | "closed"
	Author    string  `json:"author"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	ClosedAt  *string `json:"closed_at"`
}

// PullReview PR review 状态（approve | request_changes | comment）。
type PullReview struct {
	ID        int64  `json:"id"`
	Owner     string `json:"-"`
	Repo      string `json:"-"`
	Number    int64  `json:"number"`
	Reviewer  string `json:"reviewer"`
	State     string `json:"state"`
	Body      string `json:"body"`
	CommitSHA string `json:"commit_sha"`
	CreatedAt string `json:"created_at"`
}

// ReviewSummary 每个 reviewer 最新 review 的汇总统计。
type ReviewSummary struct {
	Approvals      int `json:"approvals"`
	RequestChanges int `json:"request_changes"`
}

// Comment issue/PR 下的评论（Kind: "issue" | "pull"）。
type Comment struct {
	ID        int64  `json:"id"`
	Owner     string `json:"-"`
	Repo      string `json:"-"`
	Kind      string `json:"-"`
	Number    int64  `json:"number"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// 行内评论（仅 PR；FilePath 为 nil 表示普通评论）
	FilePath *string `json:"file_path"`
	Line     *int64  `json:"line"`
	LineSide string  `json:"line_side"`
}

// Collab 仓库协作者（read=可克隆/浏览，write=可 push）
type Collab struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Username   string `json:"username"`
	Permission string `json:"permission"` // "read" | "write"
	CreatedAt  string `json:"created_at"`
}

// PullRequest 仓库内的拉取请求（同仓库分支合并，MVP：仅支持 fast-forward 合并）
type PullRequest struct {
	ID           int64   `json:"id"`
	Owner        string  `json:"-"`
	Repo         string  `json:"-"`
	Number       int64   `json:"number"`
	Title        string  `json:"title"`
	Body         string  `json:"body"`
	SourceBranch string  `json:"source_branch"`
	TargetBranch string  `json:"target_branch"`
	BaseSHA      string  `json:"base_sha"`
	HeadSHA      string  `json:"head_sha"`
	State        string  `json:"state"` // open | merged | closed
	Author       string  `json:"author"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	MergedAt     *string `json:"merged_at"`
	MergedBy     string  `json:"merged_by"`
}

// Org 组织（命名空间）：成员可把仓库 owner 设为组织名。
type Org struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Display   string `json:"display"`
	CreatedAt string `json:"created_at"`
}

// OrgMember 组织成员（owner 拥有全部管理权，member 可写组织仓库）
type OrgMember struct {
	Org      string `json:"org"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// Label 仓库 issue 标签
type Label struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
}

// Milestone 仓库里程碑
type Milestone struct {
	ID           int64  `json:"id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	State        string `json:"state"` // open | closed
	OpenIssues   int    `json:"open_issues"`
	ClosedIssues int    `json:"closed_issues"`
	CreatedAt    string `json:"created_at"`
}

type GPGKey struct {
	ID          int64  `json:"id"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}

type GPGKeyAuth struct {
	Username    string
	Fingerprint string
	Armor       string
}

type Webhook struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	URL       string `json:"url"`
	Secret    string `json:"-"` // 签名密钥，不回传
	CreatedAt string `json:"created_at"`
}

// PublicKeyAuth 用于 SSH 鉴权：公钥行 + 所属用户
type PublicKeyAuth struct {
	UserID   int64
	Username string
	Line     string `gorm:"column:public_key"`
}

// ---- Store ----

type Store struct {
	db *gorm.DB
}

// Open 打开 SQLite 数据库文件（默认后端）。
func Open(path string) (*Store, error) {
	return openGorm(gormsqlite.Open(fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)", path)))
}

// OpenDSN 按连接串选择后端：postgres://... → PostgreSQL；否则视为 SQLite 文件路径。
func OpenDSN(dsn string) (*Store, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return openGorm(postgres.Open(dsn))
	}
	return Open(dsn)
}

func openGorm(dial gorm.Dialector) (*Store, error) {
	db, err := gorm.Open(dial, &gorm.Config{
		TranslateError: true, // 唯一约束 → gorm.ErrDuplicatedKey
		Logger:         gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// DB 暴露底层 gorm.DB（供需要原生查询的特殊场景使用）。
func (s *Store) DB() *gorm.DB { return s.db }

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func isUniqueErr(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// notFoundErr 把 gorm 的查不到错误统一映射为 ErrNotFound。
func notFoundErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
