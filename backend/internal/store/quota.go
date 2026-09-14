package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Quota 是一组资源数量上限。字段值为 0 表示**不限**（默认即全 0，保持向后兼容）。
//
// 作用域与语义：
//   - 个人仓库计入 owner 用户配额；组织命名空间下的仓库计入该组织配额；
//   - 组织数与组织成员数按组织维度统计；
//   - key/令牌按用户维度统计。
//
// 覆盖规则：per-user / per-org 覆盖是**整体替换**默认配置（不是逐字段合并）。
type Quota struct {
	MaxReposPerUser    int `json:"max_repos_per_user"`
	MaxReposPerOrg     int `json:"max_repos_per_org"`
	MaxOrgsPerUser     int `json:"max_orgs_per_user"`
	MaxOrgMembers      int `json:"max_org_members"`
	MaxSSHKeysPerUser  int `json:"max_ssh_keys_per_user"`
	MaxGPGKeysPerUser  int `json:"max_gpg_keys_per_user"`
	MaxPATsPerUser     int `json:"max_pats_per_user"`
	MaxWebhooksPerRepo int `json:"max_webhooks_per_repo"`
}

// ErrQuotaExceeded 是配额超限的哨兵错误，API 层据此返回 quota_exceeded。
var ErrQuotaExceeded = errors.New("quota exceeded")

// QuotaError 描述具体触发的配额项。
type QuotaError struct {
	Scope string // user | org
	Item  string // repos | orgs | org_members | ssh_keys | gpg_keys | pats | webhooks
	Limit int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("quota exceeded: %s %s limit is %d", e.Scope, e.Item, e.Limit)
}

// Is 让 errors.Is(err, ErrQuotaExceeded) 成立。
func (e *QuotaError) Is(target error) bool { return target == ErrQuotaExceeded }

func (q Quota) normalized() Quota {
	if q.MaxReposPerUser < 0 {
		q.MaxReposPerUser = 0
	}
	if q.MaxReposPerOrg < 0 {
		q.MaxReposPerOrg = 0
	}
	if q.MaxOrgsPerUser < 0 {
		q.MaxOrgsPerUser = 0
	}
	if q.MaxOrgMembers < 0 {
		q.MaxOrgMembers = 0
	}
	if q.MaxSSHKeysPerUser < 0 {
		q.MaxSSHKeysPerUser = 0
	}
	if q.MaxGPGKeysPerUser < 0 {
		q.MaxGPGKeysPerUser = 0
	}
	if q.MaxPATsPerUser < 0 {
		q.MaxPATsPerUser = 0
	}
	if q.MaxWebhooksPerRepo < 0 {
		q.MaxWebhooksPerRepo = 0
	}
	return q
}

const (
	quotaDefaultKey = "quota:default"
	quotaUserPrefix = "quota:user:"
	quotaOrgPrefix  = "quota:org:"
)

// createMu 串行化本进程内的创建操作，缩小「校验-写入」之间的竞态窗口。
// 多实例部署下仍可能并发放行（每个实例各自计数），属已知限制。
var createMu sync.Mutex

// QuotaDefault 返回实例默认配额；未配置时全 0（不限）。
func (s *Store) QuotaDefault() Quota {
	v := s.GetSetting(quotaDefaultKey)
	if v == "" {
		return Quota{}
	}
	var q Quota
	if json.Unmarshal([]byte(v), &q) != nil {
		return Quota{}
	}
	return q.normalized()
}

// SetQuotaDefault 保存实例默认配额。
func (s *Store) SetQuotaDefault(q Quota) error {
	b, err := json.Marshal(q.normalized())
	if err != nil {
		return err
	}
	return s.SetSetting(quotaDefaultKey, string(b))
}

func (s *Store) quotaOverride(prefix, name string) (Quota, bool) {
	v := s.GetSetting(prefix + strings.ToLower(name))
	if v == "" {
		return Quota{}, false
	}
	var q Quota
	if json.Unmarshal([]byte(v), &q) != nil {
		return Quota{}, false
	}
	return q.normalized(), true
}

// QuotaForUser 返回用户生效配额（存在覆盖则整体替换默认）。
func (s *Store) QuotaForUser(username string) Quota {
	if q, ok := s.quotaOverride(quotaUserPrefix, username); ok {
		return q
	}
	return s.QuotaDefault()
}

// QuotaForOrg 返回组织生效配额（存在覆盖则整体替换默认）。
func (s *Store) QuotaForOrg(org string) Quota {
	if q, ok := s.quotaOverride(quotaOrgPrefix, org); ok {
		return q
	}
	return s.QuotaDefault()
}

// SetUserQuota 保存用户配额覆盖。
func (s *Store) SetUserQuota(username string, q Quota) error {
	b, err := json.Marshal(q.normalized())
	if err != nil {
		return err
	}
	return s.SetSetting(quotaUserPrefix+strings.ToLower(username), string(b))
}

// SetOrgQuota 保存组织配额覆盖。
func (s *Store) SetOrgQuota(org string, q Quota) error {
	b, err := json.Marshal(q.normalized())
	if err != nil {
		return err
	}
	return s.SetSetting(quotaOrgPrefix+strings.ToLower(org), string(b))
}

// DeleteUserQuota / DeleteOrgQuota 移除覆盖，恢复默认。
func (s *Store) DeleteUserQuota(username string) error {
	return s.db.Where("\"key\" = ?", quotaUserPrefix+strings.ToLower(username)).Delete(&settingRow{}).Error
}

func (s *Store) DeleteOrgQuota(org string) error {
	return s.db.Where("\"key\" = ?", quotaOrgPrefix+strings.ToLower(org)).Delete(&settingRow{}).Error
}

// QuotaOverride 是管理端展示的一条覆盖记录。
type QuotaOverride struct {
	Scope string `json:"scope"` // user | org
	Name  string `json:"name"`
	Quota Quota  `json:"quota"`
}

// ListQuotaOverrides 列出全部用户/组织配额覆盖（管理端使用）。
func (s *Store) ListQuotaOverrides() ([]QuotaOverride, error) {
	var rows []settingRow
	err := s.db.Where("\"key\" LIKE ? OR \"key\" LIKE ?", quotaUserPrefix+"%", quotaOrgPrefix+"%").
		Order("\"key\"").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []QuotaOverride{}
	for _, row := range rows {
		var scope, name string
		switch {
		case strings.HasPrefix(row.Key, quotaUserPrefix):
			scope, name = "user", strings.TrimPrefix(row.Key, quotaUserPrefix)
		case strings.HasPrefix(row.Key, quotaOrgPrefix):
			scope, name = "org", strings.TrimPrefix(row.Key, quotaOrgPrefix)
		default:
			continue
		}
		var q Quota
		if json.Unmarshal([]byte(row.Value), &q) != nil {
			continue
		}
		out = append(out, QuotaOverride{Scope: scope, Name: name, Quota: q.normalized()})
	}
	return out, nil
}

// ---- 创建前校验（调用方应在持有 createMu 时调用）----

func (s *Store) countRepos(owner string) (int64, error) {
	var n int64
	err := s.db.Model(&repoRow{}).Where("owner = ?", owner).Count(&n).Error
	return n, err
}

func (s *Store) countOwnedOrgs(username string) (int64, error) {
	var n int64
	err := s.db.Model(&orgMemberRow{}).Where("username = ? AND role = ?", username, "owner").Count(&n).Error
	return n, err
}

func (s *Store) countOrgMembers(org string) (int64, error) {
	var n int64
	err := s.db.Model(&orgMemberRow{}).Where("org = ?", org).Count(&n).Error
	return n, err
}

func (s *Store) countUserRows(table, username string) (int64, error) {
	var n int64
	sub := s.db.Model(&userRow{}).Select("id").Where("username = ?", username)
	err := s.db.Table(table).Where("user_id = (?)", sub).Count(&n).Error
	return n, err
}

func (s *Store) countWebhooks(owner, repo string) (int64, error) {
	var n int64
	err := s.db.Model(&webhookRow{}).Where("owner = ? AND repo = ?", owner, repo).Count(&n).Error
	return n, err
}

// checkRepoQuota 校验创建仓库是否超限（owner 为用户名或组织名）。
func (s *Store) checkRepoQuota(owner string) error {
	if s.IsOrg(owner) {
		q := s.QuotaForOrg(owner)
		if q.MaxReposPerOrg <= 0 {
			return nil
		}
		n, err := s.countRepos(owner)
		if err != nil {
			return err
		}
		if n >= int64(q.MaxReposPerOrg) {
			return &QuotaError{Scope: "org", Item: "repos", Limit: q.MaxReposPerOrg}
		}
		return nil
	}
	q := s.QuotaForUser(owner)
	if q.MaxReposPerUser <= 0 {
		return nil
	}
	n, err := s.countRepos(owner)
	if err != nil {
		return err
	}
	if n >= int64(q.MaxReposPerUser) {
		return &QuotaError{Scope: "user", Item: "repos", Limit: q.MaxReposPerUser}
	}
	return nil
}

func (s *Store) checkOrgQuota(username string) error {
	q := s.QuotaForUser(username)
	if q.MaxOrgsPerUser <= 0 {
		return nil
	}
	n, err := s.countOwnedOrgs(username)
	if err != nil {
		return err
	}
	if n >= int64(q.MaxOrgsPerUser) {
		return &QuotaError{Scope: "user", Item: "orgs", Limit: q.MaxOrgsPerUser}
	}
	return nil
}

func (s *Store) checkOrgMemberQuota(org string) error {
	q := s.QuotaForOrg(org)
	if q.MaxOrgMembers <= 0 {
		return nil
	}
	n, err := s.countOrgMembers(org)
	if err != nil {
		return err
	}
	if n >= int64(q.MaxOrgMembers) {
		return &QuotaError{Scope: "org", Item: "org_members", Limit: q.MaxOrgMembers}
	}
	return nil
}

func (s *Store) checkUserCountQuota(username, table, item string, limit int) error {
	if limit <= 0 {
		return nil
	}
	n, err := s.countUserRows(table, username)
	if err != nil {
		return err
	}
	if n >= int64(limit) {
		return &QuotaError{Scope: "user", Item: item, Limit: limit}
	}
	return nil
}

func (s *Store) usernameByID(id int64) (string, error) {
	var row userRow
	if err := s.db.Select("username").Where("id = ?", id).First(&row).Error; err != nil {
		return "", notFoundErr(err)
	}
	return row.Username, nil
}

// CheckPATQuota 按用户 ID 校验 PAT 数量配额（供 CreatePAT 调用）。
func (s *Store) CheckPATQuota(userID int64) error {
	username, err := s.usernameByID(userID)
	if err != nil {
		return err
	}
	return s.checkUserCountQuota(username, "pats", "pats", s.QuotaForUser(username).MaxPATsPerUser)
}

func (s *Store) checkWebhookQuota(owner, repo string) error {
	q := s.QuotaForUser(owner)
	if s.IsOrg(owner) {
		q = s.QuotaForOrg(owner)
	}
	if q.MaxWebhooksPerRepo <= 0 {
		return nil
	}
	n, err := s.countWebhooks(owner, repo)
	if err != nil {
		return err
	}
	if n >= int64(q.MaxWebhooksPerRepo) {
		return &QuotaError{Scope: "repo", Item: "webhooks", Limit: q.MaxWebhooksPerRepo}
	}
	return nil
}
