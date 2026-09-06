package store

import "errors"

// ---- PR reviews ----

// CreateReview 新增一条 PR review；同一 reviewer 重复提交插入新行（保留历史）。
func (s *Store) CreateReview(owner, repo string, number int64, reviewer, state, body, commitSHA string) (PullReview, error) {
	if state != "approve" && state != "request_changes" && state != "comment" {
		return PullReview{}, errors.New("invalid review state")
	}
	r := pullReviewRow{
		Owner: owner, Repo: repo, Number: number,
		Reviewer: reviewer, State: state, Body: body, CommitSHA: commitSHA,
		CreatedAt: now(),
	}
	if err := s.db.Create(&r).Error; err != nil {
		return PullReview{}, err
	}
	return PullReview{
		ID: r.ID, Owner: owner, Repo: repo, Number: r.Number,
		Reviewer: reviewer, State: state, Body: body, CommitSHA: commitSHA,
		CreatedAt: r.CreatedAt,
	}, nil
}

// ListReviews 列出 PR 的全部 review（含历史，升序），
// 并按每个 reviewer 最新一条汇总 approve / request_changes 计数。
func (s *Store) ListReviews(owner, repo string, number int64) ([]PullReview, ReviewSummary, error) {
	var rows []pullReviewRow
	if err := s.db.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, ReviewSummary{}, err
	}
	reviews := []PullReview{}
	latest := map[string]string{}
	for _, r := range rows {
		reviews = append(reviews, PullReview{
			ID: r.ID, Owner: r.Owner, Repo: r.Repo, Number: r.Number,
			Reviewer: r.Reviewer, State: r.State, Body: r.Body,
			CommitSHA: r.CommitSHA, CreatedAt: r.CreatedAt,
		})
		latest[r.Reviewer] = r.State // 升序遍历，最后写入即最新
	}
	var sum ReviewSummary
	for _, state := range latest {
		switch state {
		case "approve":
			sum.Approvals++
		case "request_changes":
			sum.RequestChanges++
		}
	}
	return reviews, sum, nil
}
