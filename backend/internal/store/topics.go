package store

import (
	"regexp"
	"strings"

	"gorm.io/gorm"
)

// ---- 仓库标签/话题（repo topics）----
//
// 每个仓库可挂多个 topic（小写、字母数字与连字符），用于 Explore 按标签搜索。
// 表 repoTopicRow 以 (owner, repo, topic) 为主键，topic 上有独立索引便于反查。

var topicRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,49}$`)

// MaxRepoTopics 单个仓库最多可挂的 topic 数。
const MaxRepoTopics = 20

// NormalizeTopics 校验并规范化 topic 列表：转小写、去空白/去重、保持输入顺序。
// 任一项非法或数量超限返回 ok=false。
func NormalizeTopics(topics []string) ([]string, bool) {
	out := make([]string, 0, len(topics))
	seen := map[string]bool{}
	for _, t := range topics {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if !topicRe.MatchString(t) {
			return nil, false
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxRepoTopics {
		return nil, false
	}
	return out, true
}

// SetRepoTopics 替换仓库的全部 topic（仅 owner 调用；传入列表需已规范化）。
func (s *Store) SetRepoTopics(owner, name string, topics []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row repoRow
		if err := tx.Select("id").Where("owner = ? AND name = ?", owner, name).First(&row).Error; err != nil {
			return notFoundErr(err)
		}
		if err := tx.Where("owner = ? AND repo = ?", owner, name).Delete(&repoTopicRow{}).Error; err != nil {
			return err
		}
		for _, t := range topics {
			if err := tx.Create(&repoTopicRow{Owner: owner, Repo: name, Topic: t}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListRepoTopics 返回单个仓库的 topic（按字母序）。
func (s *Store) ListRepoTopics(owner, name string) ([]string, error) {
	var rows []repoTopicRow
	if err := s.db.Where("owner = ? AND repo = ?", owner, name).Order("topic").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Topic)
	}
	return out, nil
}

// TopicsForRepos 批量取多个仓库的 topic，返回 pair(owner,name) → topics。
func (s *Store) TopicsForRepos(pairs [][2]string) map[[2]string][]string {
	out := map[[2]string][]string{}
	if len(pairs) == 0 {
		return out
	}
	want := make(map[[2]string]bool, len(pairs))
	ownerSet := map[string]bool{}
	var owners []string
	for _, p := range pairs {
		want[p] = true
		if !ownerSet[p[0]] {
			ownerSet[p[0]] = true
			owners = append(owners, p[0])
		}
	}
	var rows []repoTopicRow
	if err := s.db.Where("owner IN ?", owners).Order("topic").Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		key := [2]string{r.Owner, r.Repo}
		if want[key] {
			out[key] = append(out[key], r.Topic)
		}
	}
	return out
}

// exploreQuery 构造公开仓库查询（可选按 topic 与关键词过滤）。
func (s *Store) exploreQuery(q, topic string) *gorm.DB {
	query := s.db.Model(&repoRow{}).Where("private = ?", false)
	if topic = strings.ToLower(strings.TrimSpace(topic)); topic != "" {
		query = query.Where(
			"EXISTS (SELECT 1 FROM repo_topics rt WHERE rt.owner = repos.owner AND rt.repo = repos.name AND rt.topic = ?)",
			topic,
		)
	}
	if q = strings.TrimSpace(q); q != "" {
		pat := likePat(q)
		query = query.Where(
			"(LOWER(owner) LIKE ? OR LOWER(name) LIKE ? OR LOWER(description) LIKE ?)",
			pat, pat, pat,
		)
	}
	return query
}

// ExploreReposFiltered 分页列出公开仓库，可按 topic 与关键词过滤；limit<=0 表示不限制。
func (s *Store) ExploreReposFiltered(q, topic string, limit, offset int) ([]Repo, error) {
	query := s.exploreQuery(q, topic).Order("id DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	var rows []repoRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	repos := []Repo{}
	for _, r := range rows {
		repos = append(repos, toRepo(r))
	}
	return repos, nil
}

// CountExploreReposFiltered 公开仓库总数（与 ExploreReposFiltered 相同的过滤条件）。
func (s *Store) CountExploreReposFiltered(q, topic string) (int, error) {
	var n int64
	if err := s.exploreQuery(q, topic).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// TopicCount 标签及使用它的公开仓库数。
type TopicCount struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

// AllTopics 列出公开仓库使用过的 topic（按热度降序），供 Explore 筛选用。
func (s *Store) AllTopics(limit int) ([]TopicCount, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []struct {
		Topic string
		Cnt   int
	}
	err := s.db.Table("repo_topics").
		Select("repo_topics.topic AS topic, COUNT(*) AS cnt").
		Joins("JOIN repos ON repos.owner = repo_topics.owner AND repos.name = repo_topics.repo").
		Where("repos.private = ?", false).
		Group("repo_topics.topic").
		Order("cnt DESC, repo_topics.topic").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]TopicCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopicCount{Topic: r.Topic, Count: r.Cnt})
	}
	return out, nil
}
