package store

import (
	"regexp"
	"strings"

	"gorm.io/gorm"
)

// IssueEvent issue/PR 活动事件（详情页时间线与通知共用）。
type IssueEvent struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`   // issue | pull
	Number    int64  `json:"number"` // 宿主编号
	Actor     string `json:"actor"`
	Action    string `json:"action"` // opened | closed | reopened | edited | labeled | unlabeled | milestoned | demilestoned | pinned | unpinned | assigned | unassigned | commented
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
}

// AddIssueEvent 记录一条活动事件（best-effort，失败仅记录日志由调用方决定）。
func (s *Store) AddIssueEvent(owner, repo, kind string, number int64, actor, action, detail string) error {
	row := issueEventRow{
		Owner: owner, Repo: repo, Kind: kind, Number: number,
		Actor: actor, Action: action, Detail: detail, CreatedAt: now(),
	}
	return s.db.Create(&row).Error
}

// ListIssueEvents 按时间升序列出某 issue/PR 的活动事件。
func (s *Store) ListIssueEvents(owner, repo, kind string, number int64) ([]IssueEvent, error) {
	var rows []issueEventRow
	if err := s.db.
		Where("owner = ? AND repo = ? AND kind = ? AND number = ?", owner, repo, kind, number).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]IssueEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, IssueEvent{
			ID: r.ID, Kind: r.Kind, Number: r.Number,
			Actor: r.Actor, Action: r.Action, Detail: r.Detail, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// ---- 指派（assignees） ----

// SetIssueAssignees 全量替换 issue 负责人列表。用户名去重；不存在的 issue 返回 ErrNotFound。
func (s *Store) SetIssueAssignees(owner, repo string, number int64, usernames []string) error {
	var row issueRow
	if err := s.db.Select("id").Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		First(&row).Error; err != nil {
		return notFoundErr(err)
	}
	seen := map[string]bool{}
	rows := make([]issueAssigneeRow, 0, len(usernames))
	for _, u := range usernames {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		rows = append(rows, issueAssigneeRow{IssueID: row.ID, Username: u})
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("issue_id = ?", row.ID).Delete(&issueAssigneeRow{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// IssueAssignees 批量返回 issue 编号 → 负责人用户名列表。
func (s *Store) IssueAssignees(owner, repo string, numbers []int64) (map[int64][]string, error) {
	out := map[int64][]string{}
	if len(numbers) == 0 {
		return out, nil
	}
	type row struct {
		Number   int64
		Username string
	}
	var rows []row
	if err := s.db.Table("issue_assignees a").
		Select("i.number AS number, a.username AS username").
		Joins("JOIN issues i ON i.id = a.issue_id").
		Where("i.owner = ? AND i.repo = ? AND i.number IN ?", owner, repo, numbers).
		Order("a.username ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Number] = append(out[r.Number], r.Username)
	}
	return out, nil
}

// MentionRe 匹配正文中的 @username（限 ASCII 用户名）。要求 @ 之前不是单词字符，
// 避免把邮箱 a@b.com 误判为提及。
var MentionRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9._-])@([A-Za-z0-9][A-Za-z0-9._-]{0,254})`)

// ExtractMentions 从正文中提取 @提及的用户名（去重，保持出现顺序）。
func ExtractMentions(body string) []string {
	matches := MentionRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		u := m[1]
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// ExistingUsernames 过滤出真实存在的用户名（忽略大小写重复）。
func (s *Store) ExistingUsernames(usernames []string) []string {
	if len(usernames) == 0 {
		return nil
	}
	var rows []userRow
	if err := s.db.Select("username").Where("username IN ?", usernames).Find(&rows).Error; err != nil {
		return nil
	}
	valid := map[string]bool{}
	for _, r := range rows {
		valid[r.Username] = true
	}
	out := make([]string, 0, len(usernames))
	for _, u := range usernames {
		if valid[u] {
			out = append(out, u)
		}
	}
	return out
}

// ---- issue/PR 订阅 ----

// SubscribeIssue 订阅某个 issue/PR（幂等）。
func (s *Store) SubscribeIssue(owner, repo, kind string, number int64, username string) error {
	row := issueSubscriberRow{
		Owner: owner, Repo: repo, Kind: kind, Number: number,
		Username: username, CreatedAt: now(),
	}
	return s.db.Where(issueSubscriberRow{
		Owner: owner, Repo: repo, Kind: kind, Number: number, Username: username,
	}).FirstOrCreate(&row).Error
}

// UnsubscribeIssue 取消订阅（幂等）。
func (s *Store) UnsubscribeIssue(owner, repo, kind string, number int64, username string) error {
	return s.db.
		Where("owner = ? AND repo = ? AND kind = ? AND number = ? AND username = ?", owner, repo, kind, number, username).
		Delete(&issueSubscriberRow{}).Error
}

// IsSubscribed 报告用户是否显式订阅了某个 issue/PR。
func (s *Store) IsSubscribed(owner, repo, kind string, number int64, username string) bool {
	var n int64
	err := s.db.Model(&issueSubscriberRow{}).
		Where("owner = ? AND repo = ? AND kind = ? AND number = ? AND username = ?", owner, repo, kind, number, username).
		Count(&n).Error
	return err == nil && n > 0
}

// IssueSubscribers 返回显式订阅某 issue/PR 的用户。
func (s *Store) IssueSubscribers(owner, repo, kind string, number int64) []string {
	var rows []issueSubscriberRow
	if err := s.db.Select("username").
		Where("owner = ? AND repo = ? AND kind = ? AND number = ?", owner, repo, kind, number).
		Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Username)
	}
	return out
}

// ---- 收件人计算 ----

// IssueAuthor 返回宿主（issue/pull）的作者；不存在时返回空串。
func (s *Store) IssueAuthor(owner, repo, kind string, number int64) string {
	if kind == "pull" {
		var r pullRequestRow
		if err := s.db.Select("author").Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
			First(&r).Error; err == nil {
			return r.Author
		}
		return ""
	}
	var r issueRow
	if err := s.db.Select("author").Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		First(&r).Error; err == nil {
		return r.Author
	}
	return ""
}

// IssueCommenters 返回某 issue/PR 下所有评论者（去重）。
func (s *Store) IssueCommenters(owner, repo, kind string, number int64) []string {
	var rows []commentRow
	if err := s.db.Select("DISTINCT author").
		Where("owner = ? AND repo = ? AND kind = ? AND number = ?", owner, repo, kind, number).
		Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Author != "" {
			out = append(out, r.Author)
		}
	}
	return out
}

// IssueAssigneesByNumber 返回 issue 负责人用户名（供收件人计算）。
func (s *Store) IssueAssigneesByNumber(owner, repo string, number int64) []string {
	m, err := s.IssueAssignees(owner, repo, []int64{number})
	if err != nil {
		return nil
	}
	return m[number]
}

// IssueParticipantRecipients 计算某 issue/PR 动态的收件人：
//   - 显式 watch 仓库的用户 + 仓库所有者 / 组织成员（沿用 NotifyRecipients）
//   - 宿主作者、评论参与者、issue 负责人、显式订阅者
//   - 正文中 @提及 且真实存在的用户
//
// 始终排除 actor 本人，结果去重。
func (s *Store) IssueParticipantRecipients(owner, repo, kind string, number int64, actor string, mentions []string) []string {
	seen := map[string]bool{}
	add := func(u string) {
		if u != "" {
			seen[u] = true
		}
	}
	for _, u := range s.NotifyRecipients(owner, repo, actor) {
		add(u)
	}
	add(s.IssueAuthor(owner, repo, kind, number))
	for _, u := range s.IssueCommenters(owner, repo, kind, number) {
		add(u)
	}
	if kind == "issue" {
		for _, u := range s.IssueAssigneesByNumber(owner, repo, number) {
			add(u)
		}
	}
	for _, u := range s.IssueSubscribers(owner, repo, kind, number) {
		add(u)
	}
	for _, u := range s.ExistingUsernames(mentions) {
		add(u)
	}
	delete(seen, actor)
	out := make([]string, 0, len(seen))
	for u := range seen {
		out = append(out, u)
	}
	return out
}

// IssueCommentCounts 批量返回某宿主（issue/pull）编号 → 评论数。
func (s *Store) IssueCommentCounts(owner, repo, kind string, numbers []int64) map[int64]int {
	out := map[int64]int{}
	if len(numbers) == 0 {
		return out
	}
	type row struct {
		Number int64
		N      int
	}
	var rows []row
	_ = s.db.Table("issue_comments").
		Select("number, COUNT(*) AS n").
		Where("owner = ? AND repo = ? AND kind = ? AND number IN ?", owner, repo, kind, numbers).
		Group("number").Scan(&rows).Error
	for _, r := range rows {
		out[r.Number] = r.N
	}
	return out
}

// PullsByNumbers 批量返回同仓库内指定编号的 PR（用于「关联的 PR」）。
func (s *Store) PullsByNumbers(owner, repo string, numbers []int64) ([]PullRequest, error) {
	out := []PullRequest{}
	if len(numbers) == 0 {
		return out, nil
	}
	var rows []pullRequestRow
	if err := s.db.Where("owner = ? AND repo = ? AND number IN ?", owner, repo, numbers).
		Order("number ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, pullToDTO(r))
	}
	return out, nil
}
