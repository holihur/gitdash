package store

// pullToDTO 把 ORM 行转换为公共 DTO。
func pullToDTO(r pullRequestRow) PullRequest {
	pr := PullRequest{
		ID: r.ID, Owner: r.Owner, Repo: r.Repo, Number: r.Number,
		Title: r.Title, Body: r.Body,
		SourceBranch: r.SourceBranch, TargetBranch: r.TargetBranch,
		BaseSHA: r.BaseSHA, HeadSHA: r.HeadSHA,
		State: r.State, Author: r.Author,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, MergedBy: r.MergedBy,
	}
	if r.MergedAt != nil {
		v := *r.MergedAt
		pr.MergedAt = &v
	}
	return pr
}

func (s *Store) getPull(owner, repo string, number int64) (PullRequest, error) {
	var r pullRequestRow
	if err := s.db.Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		First(&r).Error; err != nil {
		return PullRequest{}, notFoundErr(err)
	}
	return pullToDTO(r), nil
}

func (s *Store) CreatePull(owner, repo, author, title, body, source, target, baseSHA, headSHA string) (PullRequest, error) {
	var pr PullRequest
	now := now()
	var err error
	// 号码在同一仓库内递增；并发冲突时重试（UNIQUE(owner, repo, number)）
	for attempt := 0; attempt < 5; attempt++ {
		var max int64
		if e := s.db.Raw(`SELECT COALESCE(MAX(number), 0) + 1 FROM pull_requests WHERE owner = ? AND repo = ?`,
			owner, repo).Scan(&max).Error; e != nil {
			return pr, e
		}
		r := pullRequestRow{
			Owner: owner, Repo: repo, Number: max,
			Title: title, Body: body,
			SourceBranch: source, TargetBranch: target,
			BaseSHA: baseSHA, HeadSHA: headSHA, State: "open", Author: author,
			CreatedAt: now, UpdatedAt: now,
		}
		e := s.db.Create(&r).Error
		if e == nil {
			return PullRequest{
				ID: r.ID, Owner: owner, Repo: repo, Number: r.Number,
				Title: title, Body: body, SourceBranch: source, TargetBranch: target,
				BaseSHA: baseSHA, HeadSHA: headSHA, State: "open", Author: author,
				CreatedAt: now, UpdatedAt: now,
			}, nil
		}
		if !isUniqueErr(e) {
			return pr, e
		}
		err = e
	}
	return pr, err
}

// GetPullIssue 按仓库内序号取 issue（供 label/milestone 更新后回读）。
func (s *Store) GetPullIssue(owner, repo string, number int64) (Issue, error) {
	return s.getIssue(owner, repo, number)
}

func (s *Store) GetPull(owner, repo string, number int64) (PullRequest, error) {
	return s.getPull(owner, repo, number)
}

// ListPulls 分页列出 PR；limit<=0 表示不限制。
func (s *Store) ListPulls(owner, repo, state string, limit, offset int) ([]PullRequest, error) {
	q := s.db.Model(&pullRequestRow{}).Where("owner = ? AND repo = ?", owner, repo)
	if state != "" {
		q = q.Where("state = ?", state)
	}
	q = q.Order("state = 'open' DESC, number DESC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	var rows []pullRequestRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []PullRequest{}
	for _, r := range rows {
		out = append(out, pullToDTO(r))
	}
	return out, nil
}

// CountPulls 仓库 PR 总数（state 为空时统计全部）。
func (s *Store) CountPulls(owner, repo, state string) (int, error) {
	q := s.db.Model(&pullRequestRow{}).Where("owner = ? AND repo = ?", owner, repo)
	if state != "" {
		q = q.Where("state = ?", state)
	}
	var n int64
	err := q.Count(&n).Error
	return int(n), err
}

// SetPullState 关闭/重开（不改变 merged 状态）。
func (s *Store) SetPullState(owner, repo string, number int64, state string) (PullRequest, error) {
	res := s.db.Model(&pullRequestRow{}).
		Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		Updates(map[string]any{"state": state, "updated_at": now()})
	if res.Error != nil {
		return PullRequest{}, res.Error
	}
	if res.RowsAffected == 0 {
		return PullRequest{}, ErrNotFound
	}
	return s.getPull(owner, repo, number)
}

// MarkPullMerged 记录合并结果（fast-forward 后 target 指向 headSHA）。
func (s *Store) MarkPullMerged(owner, repo string, number int64, headSHA, mergedBy string) (PullRequest, error) {
	now := now()
	res := s.db.Model(&pullRequestRow{}).
		Where("owner = ? AND repo = ? AND number = ?", owner, repo, number).
		Updates(map[string]any{
			"state": "merged", "head_sha": headSHA, "merged_by": mergedBy,
			"merged_at": now, "updated_at": now,
		})
	if res.Error != nil {
		return PullRequest{}, res.Error
	}
	if res.RowsAffected == 0 {
		return PullRequest{}, ErrNotFound
	}
	return s.getPull(owner, repo, number)
}
