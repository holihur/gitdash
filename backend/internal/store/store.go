package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

type UserAuth struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    string
	MFASecret    string // 空 = 未启用/无待激活 secret
	MFAEnabled   bool
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
}

// Mirror 仓库 push 镜像目标（同步到 GitHub/GitLab 等第三方）。
type Mirror struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	URL        string `json:"url"`
	PrivateKey string `json:"-"` // 认证私钥，不回传
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

// Collab 仓库协作者（read=可克隆/浏览，write=可 push）
type Collab struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Username   string `json:"username"`
	Permission string `json:"permission"` // "read" | "write"
	CreatedAt  string `json:"created_at"`
}

// Webhook push 事件回调配置

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
	Line     string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
