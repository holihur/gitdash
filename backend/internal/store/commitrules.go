package store

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RepoCommitRules 仓库提交身份校验规则：校验 push 引入提交的作者/提交者
// 姓名与邮箱格式。空 pattern 表示该项不校验；两项都为空 = 不限制（无规则行）。
type RepoCommitRules struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	NamePattern  string `json:"name_pattern"`
	EmailPattern string `json:"email_pattern"`
	CreatedAt    string `json:"created_at"`
}

type repoCommitRuleRow struct {
	Owner        string `gorm:"primaryKey;size:255"`
	Repo         string `gorm:"primaryKey;size:255"`
	NamePattern  string `gorm:"not null;default:'';size:512"`
	EmailPattern string `gorm:"not null;default:'';size:512"`
	CreatedAt    string `gorm:"not null"`
}

func (repoCommitRuleRow) TableName() string { return "repo_commit_rules" }

func commitRulesFromRow(r repoCommitRuleRow) RepoCommitRules {
	return RepoCommitRules{
		Owner: r.Owner, Repo: r.Repo,
		NamePattern: r.NamePattern, EmailPattern: r.EmailPattern, CreatedAt: r.CreatedAt,
	}
}

// GetRepoCommitRules 返回仓库提交身份规则；无规则时 ok=false。
func (s *Store) GetRepoCommitRules(owner, repo string) (RepoCommitRules, bool, error) {
	var row repoCommitRuleRow
	err := s.db.Where("owner = ? AND repo = ?", owner, repo).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RepoCommitRules{}, false, nil
	}
	if err != nil {
		return RepoCommitRules{}, false, err
	}
	return commitRulesFromRow(row), true, nil
}

// SetRepoCommitRules 新增/覆盖规则；两项都为空时等同删除（关闭限制）。
func (s *Store) SetRepoCommitRules(owner, repo, namePattern, emailPattern string) error {
	row := repoCommitRuleRow{
		Owner: owner, Repo: repo,
		NamePattern: namePattern, EmailPattern: emailPattern, CreatedAt: now(),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "repo"}},
		DoUpdates: clause.AssignmentColumns([]string{"name_pattern", "email_pattern"}),
	}).Create(&row).Error
}

// DeleteRepoCommitRules 删除规则（关闭限制）。
func (s *Store) DeleteRepoCommitRules(owner, repo string) error {
	return s.db.Where("owner = ? AND repo = ?", owner, repo).Delete(&repoCommitRuleRow{}).Error
}
